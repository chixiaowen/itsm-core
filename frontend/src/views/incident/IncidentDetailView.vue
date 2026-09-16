<template>
  <div v-loading="loading" class="page-container">
    <el-page-header :content="incident?.code || '事件详情'" @back="router.back()" />

    <el-card v-if="incident" shadow="never" class="mt-16">
      <div class="detail-header">
        <span class="detail-title">{{ incident.title }}</span>
        <StatusTag :status="incident.status" />
        <PriorityTag :priority="incident.priority" />
        <el-tag v-if="incident.priority_overridden" type="warning" size="small">优先级已覆盖</el-tag>
        <el-tag type="danger" size="small">升级 L{{ incident.escalation_level }}</el-tag>
        <div class="header-actions">
          <el-button
            v-for="action in availableActions"
            :key="action.action"
            :type="action.danger ? 'danger' : 'primary'"
            size="small"
            @click="onActionClick(action)"
          >
            {{ action.label }}
          </el-button>
          <el-button
            v-if="canConvert"
            type="success"
            size="small"
            @click="convertToTicket"
          >
            一键转工单
          </el-button>
          <el-button v-if="canOverride" type="warning" size="small" @click="overrideDialog = true">
            覆盖优先级
          </el-button>
        </div>
      </div>
    </el-card>

    <el-row v-if="incident" :gutter="16" class="mt-16">
      <el-col :xs="24" :md="8">
        <el-card shadow="never">
          <template #header><span class="card-title">影响度 × 紧急度 → 优先级</span></template>
          <table class="matrix">
            <thead>
              <tr>
                <th>影响 \ 紧急</th>
                <th>高</th>
                <th>中</th>
                <th>低</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="impact in impacts" :key="impact">
                <th>{{ IMPACT_LABELS[impact] }}</th>
                <td
                  v-for="urgency in urgencies"
                  :key="urgency"
                  :class="{ active: incident.impact === impact && incident.urgency === urgency }"
                >
                  {{ computePriority(impact, urgency) }}
                </td>
              </tr>
            </tbody>
          </table>
        </el-card>

        <el-card shadow="never" class="mt-16">
          <template #header><span class="card-title">基本信息</span></template>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="编号">{{ incident.code }}</el-descriptions-item>
            <el-descriptions-item label="影响度">{{ IMPACT_LABELS[incident.impact] }}</el-descriptions-item>
            <el-descriptions-item label="紧急度">{{ URGENCY_LABELS[incident.urgency] }}</el-descriptions-item>
            <el-descriptions-item label="上报人">{{ incident.reporter_id }}</el-descriptions-item>
            <el-descriptions-item label="处理人">{{ incident.assignee_id ?? '未指派' }}</el-descriptions-item>
            <el-descriptions-item label="关联工单">{{ incident.ticket_id ?? '未关联' }}</el-descriptions-item>
            <el-descriptions-item label="发生时间">{{ formatDateTime(incident.occurred_at) }}</el-descriptions-item>
            <el-descriptions-item label="创建时间">{{ formatDateTime(incident.created_at) }}</el-descriptions-item>
            <el-descriptions-item label="描述"><pre class="pre-wrap">{{ incident.description || '-' }}</pre></el-descriptions-item>
            <el-descriptions-item v-if="incident.solution" label="解决方案">
              <pre class="pre-wrap">{{ incident.solution }}</pre>
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>

      <el-col :xs="24" :md="16">
        <el-card shadow="never">
          <template #header><span class="card-title">处理时间线</span></template>
          <Timeline :items="timelineItems" />
        </el-card>
      </el-col>
    </el-row>

    <!-- 通用流转 -->
    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="480px">
      <el-form label-width="90px">
        <el-form-item v-if="dialogNeeds.assignee" label="指派给">
          <UserSelect
            v-model="transitionForm.assignee_id"
            role="agent,resolver"
            placeholder="请选择处理人（坐席 / 工程师）"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item v-if="dialogNeeds.solution" label="解决方案">
          <el-input v-model="transitionForm.solution" type="textarea" :rows="3" placeholder="必填" />
        </el-form-item>
        <el-form-item v-if="dialogNeeds.reason" label="原因/说明">
          <el-input v-model="transitionForm.reason" type="textarea" :rows="3" placeholder="必填" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitTransition">确认</el-button>
      </template>
    </el-dialog>

    <!-- 升级 -->
    <el-dialog v-model="escalateDialog" title="事件升级" width="480px">
      <el-form label-width="100px">
        <el-form-item label="升级类型">
          <el-radio-group v-model="escalateForm.type">
            <el-radio value="functional">功能升级</el-radio>
            <el-radio value="hierarchical">层级升级</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="升级原因">
          <el-input v-model="escalateForm.reason" type="textarea" :rows="3" placeholder="必填" />
        </el-form-item>
        <el-form-item label="转派给">
          <UserSelect
            v-model="escalateForm.to_assignee_id"
            role="agent,resolver"
            placeholder="可选：选择接手人（坐席 / 工程师）"
            style="width: 100%"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="escalateDialog = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitEscalate">确认升级</el-button>
      </template>
    </el-dialog>

    <!-- 覆盖优先级 -->
    <el-dialog v-model="overrideDialog" title="人工覆盖优先级" width="420px">
      <el-select v-model="overridePriority" style="width: 100%">
        <el-option v-for="opt in PRIORITY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <template #footer>
        <el-button @click="overrideDialog = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitOverride">确认</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import StatusTag from '@/components/StatusTag.vue'
import PriorityTag from '@/components/PriorityTag.vue'
import Timeline from '@/components/Timeline.vue'
import UserSelect from '@/components/UserSelect.vue'
import { useIncidentStore } from '@/stores/incident'
import { useUserStore } from '@/stores/user'
import { useAuditTimeline } from '@/composables/useAuditTimeline'
import {
  formatDateTime,
  PRIORITY_OPTIONS,
  sortTimeline,
  type ActionDef,
  type Priority,
  type TimelineItem
} from '@/types/common'
import {
  computePriority,
  IMPACT_LABELS,
  incidentActions,
  INCIDENT_ACTION_LABELS,
  URGENCY_LABELS,
  type IncidentAction,
  type ImpactLevel,
  type UrgencyLevel
} from '@/types/incident'

interface Escalation {
  id: number
  level: number
  type: string
  reason: string
  created_at: string
  actor_id: number
}

/** 事件详情响应可能携带 escalations 字段（后端 GET /incidents/:id 含升级历史） */
interface IncidentWithEscalations {
  escalations?: Escalation[]
}

const route = useRoute()
const router = useRouter()
const store = useIncidentStore()
const userStore = useUserStore()

const incidentId = computed(() => Number(route.params.id))
const incident = computed(() => store.current)
const loading = computed(() => store.loading)

const impacts: ImpactLevel[] = ['high', 'medium', 'low']
const urgencies: UrgencyLevel[] = ['high', 'medium', 'low']

const dialogVisible = ref(false)
const escalateDialog = ref(false)
const overrideDialog = ref(false)
const submitting = ref(false)
const currentAction = ref<ActionDef<IncidentAction> | null>(null)
const overridePriority = ref<Priority>('P3')

const transitionForm = reactive<{ solution?: string; reason?: string; assignee_id?: number }>({})
const escalateForm = reactive<{ type: 'functional' | 'hierarchical'; reason: string; to_assignee_id?: number }>({
  type: 'functional',
  reason: '',
  to_assignee_id: undefined
})

/** 状态流转时间线（审计日志）：403 时优雅降级隐藏 */
const audit = useAuditTimeline({
  entityType: computed(() => 'incident'),
  entityId: computed(() => incidentId.value || null),
  actionLabels: INCIDENT_ACTION_LABELS
})

const availableActions = computed<ActionDef<IncidentAction>[]>(() =>
  incident.value ? incidentActions(incident.value.status, userStore.role as string) : []
)

const canConvert = computed(
  () =>
    Boolean(incident.value) &&
    ['triage', 'in_progress'].includes(incident.value!.status) &&
    !incident.value!.ticket_id &&
    ['agent', 'resolver', 'admin'].includes(userStore.role as string)
)

const canOverride = computed(
  () =>
    ['agent', 'resolver', 'problem_manager', 'change_manager', 'admin'].includes(userStore.role as string) &&
    Boolean(incident.value)
)

const dialogNeeds = computed(() => {
  const action = currentAction.value?.action
  return {
    assignee: action === 'confirm',
    solution: action === 'resolve' || action === 'false_positive',
    reason: action === 'cancel' || action === 'pending'
  }
})

const dialogTitle = computed(() => (currentAction.value ? `执行操作：${currentAction.value.label}` : '执行操作'))

const escalationTimeline = computed<TimelineItem[]>(() => {
  const list = (incident.value as unknown as IncidentWithEscalations)?.escalations ?? []
  return list.map((item) => ({
    id: `esc-${item.id}`,
    time: item.created_at,
    title: `升级至 L${item.level}（${item.type === 'functional' ? '功能升级' : '层级升级'}）`,
    type: 'danger',
    content: item.reason
  }))
})

/** 处理时间线：升级历史 + 状态流转（审计日志）；无权限时仅显示升级历史 */
const timelineItems = computed<TimelineItem[]>(() => {
  const items: TimelineItem[] = [...escalationTimeline.value]
  if (!audit.forbidden.value) {
    audit.items.value.forEach((item) => items.push(item))
  }
  return sortTimeline(items)
})

async function loadDetail(): Promise<void> {
  await store.fetchDetail(incidentId.value)
  await Promise.all([store.fetchMatrix().catch(() => undefined), audit.reload()])
}

function onActionClick(action: ActionDef<IncidentAction>): void {
  currentAction.value = action
  if (action.action === 'escalate') {
    escalateForm.type = 'functional'
    escalateForm.reason = ''
    escalateForm.to_assignee_id = undefined
    escalateDialog.value = true
    return
  }
  transitionForm.solution = ''
  transitionForm.reason = ''
  transitionForm.assignee_id = incident.value?.assignee_id ?? undefined
  dialogVisible.value = true
}

async function submitTransition(): Promise<void> {
  if (!currentAction.value) return
  if (dialogNeeds.value.assignee && !transitionForm.assignee_id) {
    ElMessage.warning('请选择处理人')
    return
  }
  if (dialogNeeds.value.solution && !transitionForm.solution) {
    ElMessage.warning('请填写解决方案')
    return
  }
  if (dialogNeeds.value.reason && !transitionForm.reason) {
    ElMessage.warning('请填写原因')
    return
  }
  submitting.value = true
  try {
    await store.transition(incidentId.value, {
      action: currentAction.value.action,
      solution: transitionForm.solution,
      reason: transitionForm.reason,
      assignee_id: transitionForm.assignee_id
    })
    ElMessage.success('操作成功')
    dialogVisible.value = false
    await loadDetail()
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

async function submitEscalate(): Promise<void> {
  if (!escalateForm.reason) {
    ElMessage.warning('请填写升级原因')
    return
  }
  submitting.value = true
  try {
    await store.escalate(incidentId.value, { ...escalateForm })
    ElMessage.success('升级成功')
    escalateDialog.value = false
    await loadDetail()
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

async function submitOverride(): Promise<void> {
  submitting.value = true
  try {
    await store.overridePriority(incidentId.value, overridePriority.value)
    ElMessage.success('优先级已覆盖')
    overrideDialog.value = false
    await loadDetail()
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

async function convertToTicket(): Promise<void> {
  try {
    const ticket = await store.convertToTicket(incidentId.value)
    ElMessage.success(`已生成工单：${ticket.code}`)
    await router.push(`/tickets/${ticket.id}`)
  } catch {
    /* handled by interceptor */
  }
}

onMounted(() => {
  void loadDetail()
})
</script>

<style scoped>
.header-actions {
  margin-left: auto;
  display: flex;
  gap: 8px;
}
.pre-wrap {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: inherit;
}
.matrix {
  width: 100%;
  border-collapse: collapse;
  text-align: center;
}
.matrix th,
.matrix td {
  border: 1px solid #ebeef5;
  padding: 8px 4px;
  font-size: 13px;
}
.matrix thead th {
  background-color: #f5f7fa;
}
.matrix td.active {
  background-color: #409eff;
  color: #fff;
  font-weight: 700;
}
</style>
