<template>
  <div v-loading="loading" class="page-container">
    <el-page-header :content="change?.code || '变更详情'" @back="router.back()" />

    <el-card v-if="change" shadow="never" class="mt-16">
      <div class="detail-header">
        <span class="detail-title">{{ change.title }}</span>
        <StatusTag :status="change.status" />
        <StatusTag :status="change.risk_level" />
        <el-tag type="info" size="small">{{ CHANGE_TYPE_LABELS[change.change_type] }}</el-tag>
        <el-tag v-if="change.pre_authorized" type="success" size="small">已预授权</el-tag>
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
        </div>
      </div>
    </el-card>

    <el-row v-if="change" :gutter="16" class="mt-16">
      <el-col :xs="24" :md="14">
        <el-card shadow="never">
          <template #header><span class="card-title">变更信息</span></template>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="编号">{{ change.code }}</el-descriptions-item>
            <el-descriptions-item label="类型">{{ CHANGE_TYPE_LABELS[change.change_type] }}</el-descriptions-item>
            <el-descriptions-item label="风险等级"><StatusTag :status="change.risk_level" /></el-descriptions-item>
            <el-descriptions-item label="申请人">{{ change.requester_id }}</el-descriptions-item>
            <el-descriptions-item label="变更经理">{{ change.manager_id ?? '-' }}</el-descriptions-item>
            <el-descriptions-item label="窗口开始">{{ formatDateTime(change.window_start) }}</el-descriptions-item>
            <el-descriptions-item label="窗口结束">{{ formatDateTime(change.window_end) }}</el-descriptions-item>
            <el-descriptions-item label="创建时间">{{ formatDateTime(change.created_at) }}</el-descriptions-item>
          </el-descriptions>
          <div class="section">
            <div class="card-title">影响分析</div>
            <pre class="pre-wrap">{{ change.impact_analysis || '-' }}</pre>
          </div>
          <div class="section">
            <div class="card-title">实施计划</div>
            <pre class="pre-wrap">{{ change.plan || '-' }}</pre>
          </div>
          <div class="section">
            <div class="card-title">回滚方案</div>
            <pre class="pre-wrap">{{ change.rollback_plan || '-' }}</pre>
          </div>
          <div v-if="change.implement_result" class="section">
            <div class="card-title">实施结论</div>
            <pre class="pre-wrap">{{ change.implement_result }}</pre>
          </div>
          <div v-if="change.rollback_reason" class="section">
            <div class="card-title">回滚原因</div>
            <pre class="pre-wrap">{{ change.rollback_reason }}</pre>
          </div>
        </el-card>
      </el-col>

      <el-col :xs="24" :md="10">
        <el-card shadow="never">
          <template #header><span class="card-title">CAB 审批记录</span></template>
          <el-table :data="store.approvals" size="small">
            <el-table-column prop="approver_id" label="审批人" width="90" />
            <el-table-column label="决议" width="100">
              <template #default="{ row }">
                <el-tag v-if="row.decision === 'approved'" type="success" size="small">通过</el-tag>
                <el-tag v-else-if="row.decision === 'rejected'" type="danger" size="small">驳回</el-tag>
                <el-tag v-else type="info" size="small">待审</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="comment" label="意见" show-overflow-tooltip />
            <template #empty><el-empty description="暂无审批记录" /></template>
          </el-table>
        </el-card>
      </el-col>
    </el-row>

    <el-card v-if="change" shadow="never" class="mt-16">
      <template #header><span class="card-title">处理时间线</span></template>
      <Timeline :items="timelineItems" />
    </el-card>

    <!-- 普通流转 -->
    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="500px">
      <el-form label-width="110px">
        <el-form-item v-if="dialogNeeds.result" label="实施结论">
          <el-input v-model="transitionForm.result" type="textarea" :rows="3" placeholder="必填" />
        </el-form-item>
        <el-form-item v-if="dialogNeeds.reason" label="原因/说明">
          <el-input v-model="transitionForm.reason" type="textarea" :rows="3" placeholder="必填" />
        </el-form-item>
        <el-form-item v-if="dialogNeeds.window" label="变更窗口">
          <el-date-picker
            v-model="windowRange"
            type="datetimerange"
            range-separator="至"
            start-placeholder="开始"
            end-placeholder="结束"
            style="width: 100%"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitTransition">确认</el-button>
      </template>
    </el-dialog>

    <!-- CAB 审批 -->
    <el-dialog v-model="approvalDialog" :title="approvalTitle" width="500px">
      <el-form label-width="90px">
        <el-form-item label="审批决议">
          <el-radio-group v-model="approvalForm.decision">
            <el-radio value="approved">通过</el-radio>
            <el-radio value="rejected">驳回</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="审批意见">
          <el-input v-model="approvalForm.comment" type="textarea" :rows="3" placeholder="必填" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="approvalDialog = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitApproval">提交审批</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import StatusTag from '@/components/StatusTag.vue'
import Timeline from '@/components/Timeline.vue'
import { useChangeStore } from '@/stores/change'
import { useUserStore } from '@/stores/user'
import { useAuditTimeline } from '@/composables/useAuditTimeline'
import { formatDateTime, sortTimeline, type ActionDef, type TimelineItem } from '@/types/common'
import {
  changeActions,
  CHANGE_ACTION_LABELS,
  CHANGE_TYPE_LABELS,
  type ApprovalDecision,
  type ChangeAction
} from '@/types/change'

const route = useRoute()
const router = useRouter()
const store = useChangeStore()
const userStore = useUserStore()

const changeId = computed(() => Number(route.params.id))
const change = computed(() => store.current)
const loading = computed(() => store.loading)

const dialogVisible = ref(false)
const approvalDialog = ref(false)
const submitting = ref(false)
const currentAction = ref<ActionDef<ChangeAction> | null>(null)
const windowRange = ref<[string, string] | null>(null)

const transitionForm = reactive<{ result?: string; reason?: string }>({})
const approvalForm = reactive<{ decision: ApprovalDecision; comment: string }>({
  decision: 'approved',
  comment: ''
})

const availableActions = computed<ActionDef<ChangeAction>[]>(() =>
  change.value ? changeActions(change.value.status, userStore.role as string) : []
)

const dialogNeeds = computed(() => {
  const action = currentAction.value?.action
  return {
    result: action === 'complete',
    reason: action === 'rollback' || action === 'reject' || action === 'cancel' || action === 'close',
    window: action === 'schedule'
  }
})

const dialogTitle = computed(() => (currentAction.value ? `执行操作：${currentAction.value.label}` : '执行操作'))
const approvalTitle = computed(() =>
  currentAction.value ? `CAB 审批：${currentAction.value.label}` : 'CAB 审批'
)

/** 状态流转时间线（审计日志）：403 时优雅降级隐藏 */
const audit = useAuditTimeline({
  entityType: computed(() => 'change'),
  entityId: computed(() => changeId.value || null),
  actionLabels: CHANGE_ACTION_LABELS
})

const timelineItems = computed<TimelineItem[]>(() =>
  audit.forbidden.value ? [] : sortTimeline(audit.items.value)
)

function onActionClick(action: ActionDef<ChangeAction>): void {
  currentAction.value = action
  if (action.action === 'approve' || action.action === 'reject') {
    approvalForm.decision = action.action === 'approve' ? 'approved' : 'rejected'
    approvalForm.comment = ''
    approvalDialog.value = true
    return
  }
  transitionForm.result = ''
  transitionForm.reason = ''
  windowRange.value = null
  dialogVisible.value = true
}

async function submitTransition(): Promise<void> {
  if (!currentAction.value) return
  if (dialogNeeds.value.result && !transitionForm.result) {
    ElMessage.warning('请填写实施结论')
    return
  }
  if (dialogNeeds.value.reason && !transitionForm.reason) {
    ElMessage.warning('请填写原因')
    return
  }
  const payload = {
    action: currentAction.value.action,
    result: transitionForm.result,
    reason: transitionForm.reason,
    window_start: windowRange.value?.[0],
    window_end: windowRange.value?.[1]
  }
  submitting.value = true
  try {
    await store.transition(changeId.value, payload)
    ElMessage.success('操作成功')
    dialogVisible.value = false
    await loadDetail()
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

async function submitApproval(): Promise<void> {
  if (!approvalForm.comment) {
    ElMessage.warning('请填写审批意见')
    return
  }
  submitting.value = true
  try {
    await store.approve(changeId.value, { ...approvalForm })
    ElMessage.success('审批已提交')
    approvalDialog.value = false
    await loadDetail()
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

async function loadDetail(): Promise<void> {
  await store.fetchDetail(changeId.value)
  await Promise.all([
    store.fetchApprovals(changeId.value).catch(() => undefined),
    audit.reload()
  ])
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
.section {
  margin-top: 16px;
}
</style>
