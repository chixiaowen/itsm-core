<template>
  <div v-loading="loading" class="page-container">
    <el-page-header :content="problem?.code || '问题详情'" @back="router.back()" />

    <el-card v-if="problem" shadow="never" class="mt-16">
      <div class="detail-header">
        <span class="detail-title">{{ problem.title }}</span>
        <StatusTag :status="problem.status" />
        <el-tag type="info" size="small">{{ PROBLEM_SOURCE_LABELS[problem.source] }}</el-tag>
        <div class="header-actions">
          <el-button
            v-for="action in availableActions"
            :key="action.action"
            :type="action.danger ? 'danger' : 'primary'"
            size="small"
            @click="openDialog(action)"
          >
            {{ action.label }}
          </el-button>
        </div>
      </div>
    </el-card>

    <el-row v-if="problem" :gutter="16" class="mt-16">
      <el-col :xs="24" :md="12">
        <el-card shadow="never">
          <template #header><span class="card-title">RCA 根因分析</span></template>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="现象"><pre class="pre-wrap">{{ problem.symptom || '-' }}</pre></el-descriptions-item>
            <el-descriptions-item label="分析过程"><pre class="pre-wrap">{{ problem.analysis || '-' }}</pre></el-descriptions-item>
            <el-descriptions-item label="根本原因"><pre class="pre-wrap">{{ problem.root_cause || '-' }}</pre></el-descriptions-item>
            <el-descriptions-item label="临时规避"><pre class="pre-wrap">{{ problem.workaround || '-' }}</pre></el-descriptions-item>
            <el-descriptions-item label="无需变更原因"><pre class="pre-wrap">{{ problem.no_change_reason || '-' }}</pre></el-descriptions-item>
          </el-descriptions>
          <el-button
            v-if="canEdit"
            type="primary"
            plain
            class="mt-8"
            @click="rcaDialog = true"
          >
            编辑 RCA
          </el-button>
        </el-card>
      </el-col>

      <el-col :xs="24" :md="12">
        <el-card shadow="never">
          <template #header><span class="card-title">关联信息</span></template>
          <div class="card-title">关联事件（{{ problem.incident_ids?.length ?? 0 }}）</div>
          <el-space wrap>
            <el-tag v-for="id in problem.incident_ids || []" :key="id" type="info">事件 #{{ id }}</el-tag>
            <span v-if="!problem.incident_ids?.length" class="text-muted">无关联事件</span>
          </el-space>
          <div class="card-title mt-16">关联变更（{{ problem.change_ids?.length ?? 0 }}）</div>
          <el-space wrap>
            <el-tag v-for="id in problem.change_ids || []" :key="id" type="success">变更 #{{ id }}</el-tag>
            <span v-if="!problem.change_ids?.length" class="text-muted">无关联变更</span>
          </el-space>
        </el-card>
      </el-col>
    </el-row>

    <el-card v-if="problem" shadow="never" class="mt-16">
      <template #header><span class="card-title">处理时间线</span></template>
      <Timeline :items="timelineItems" />
    </el-card>

    <!-- 状态流转 -->
    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="500px">
      <el-form label-width="110px">
        <el-form-item v-if="dialogNeeds.workaround" label="规避方案">
          <el-input v-model="transitionForm.workaround" type="textarea" :rows="3" placeholder="必填" />
        </el-form-item>
        <el-form-item v-if="dialogNeeds.noChangeReason" label="无需变更原因">
          <el-input v-model="transitionForm.no_change_reason" type="textarea" :rows="3" placeholder="与关联变更二选一" />
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

    <!-- RCA 编辑 -->
    <el-dialog v-model="rcaDialog" title="编辑 RCA" width="560px">
      <el-form label-width="100px">
        <el-form-item label="现象"><el-input v-model="rcaForm.symptom" type="textarea" :rows="2" /></el-form-item>
        <el-form-item label="分析过程"><el-input v-model="rcaForm.analysis" type="textarea" :rows="3" /></el-form-item>
        <el-form-item label="根本原因"><el-input v-model="rcaForm.root_cause" type="textarea" :rows="3" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="rcaDialog = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitRca">保存</el-button>
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
import { useProblemStore } from '@/stores/problem'
import { useUserStore } from '@/stores/user'
import { useAuditTimeline } from '@/composables/useAuditTimeline'
import { sortTimeline, type ActionDef, type TimelineItem } from '@/types/common'
import {
  problemActions,
  PROBLEM_ACTION_LABELS,
  PROBLEM_SOURCE_LABELS,
  type ProblemAction
} from '@/types/problem'

const route = useRoute()
const router = useRouter()
const store = useProblemStore()
const userStore = useUserStore()

const problemId = computed(() => Number(route.params.id))
const problem = computed(() => store.current)
const loading = computed(() => store.loading)

const dialogVisible = ref(false)
const rcaDialog = ref(false)
const submitting = ref(false)
const currentAction = ref<ActionDef<ProblemAction> | null>(null)

const transitionForm = reactive<{
  workaround?: string
  no_change_reason?: string
  reason?: string
}>({})

const rcaForm = reactive({ symptom: '', analysis: '', root_cause: '' })

/** 状态流转时间线（审计日志）：403 时优雅降级隐藏 */
const audit = useAuditTimeline({
  entityType: computed(() => 'problem'),
  entityId: computed(() => problemId.value || null),
  actionLabels: PROBLEM_ACTION_LABELS
})

const timelineItems = computed<TimelineItem[]>(() =>
  audit.forbidden.value ? [] : sortTimeline(audit.items.value)
)

const availableActions = computed<ActionDef<ProblemAction>[]>(() =>
  problem.value ? problemActions(problem.value.status, userStore.role as string) : []
)

const canEdit = computed(() => ['problem_manager', 'admin'].includes(userStore.role as string))

const dialogNeeds = computed(() => {
  const action = currentAction.value?.action
  return {
    workaround: action === 'mark_known_error' || action === 'update_workaround',
    noChangeReason: action === 'resolve',
    reason: action === 'cancel' || action === 'recur'
  }
})

const dialogTitle = computed(() => (currentAction.value ? `执行操作：${currentAction.value.label}` : '执行操作'))

function openDialog(action: ActionDef<ProblemAction>): void {
  currentAction.value = action
  transitionForm.workaround = problem.value?.workaround ?? ''
  transitionForm.no_change_reason = ''
  transitionForm.reason = ''
  dialogVisible.value = true
}

async function submitTransition(): Promise<void> {
  if (!currentAction.value) return
  if (dialogNeeds.value.workaround && !transitionForm.workaround) {
    ElMessage.warning('请填写规避方案')
    return
  }
  if (dialogNeeds.value.reason && !transitionForm.reason) {
    ElMessage.warning('请填写原因')
    return
  }
  submitting.value = true
  try {
    await store.transition(problemId.value, {
      action: currentAction.value.action,
      workaround: transitionForm.workaround,
      no_change_reason: transitionForm.no_change_reason,
      reason: transitionForm.reason
    })
    ElMessage.success('操作成功')
    dialogVisible.value = false
    await store.fetchDetail(problemId.value)
    await audit.reload()
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

async function submitRca(): Promise<void> {
  submitting.value = true
  try {
    await store.rca(problemId.value, { ...rcaForm })
    ElMessage.success('已保存')
    rcaDialog.value = false
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  const data = await store.fetchDetail(problemId.value)
  rcaForm.symptom = data.symptom
  rcaForm.analysis = data.analysis
  rcaForm.root_cause = data.root_cause
  await audit.reload()
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
</style>
