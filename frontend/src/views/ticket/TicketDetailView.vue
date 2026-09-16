<template>
  <div v-loading="loading" class="page-container">
    <el-page-header :content="ticket?.code || '工单详情'" @back="router.back()" />

    <el-card v-if="ticket" shadow="never" class="mt-16">
      <div class="detail-header">
        <span class="detail-title">{{ ticket.title }}</span>
        <StatusTag :status="ticket.status" />
        <PriorityTag :priority="ticket.priority" />
        <SlaCountdown
          :sla-status="ticket.sla_status"
          :due-at="ticket.resolve_due_at"
          :finished="['resolved', 'closed', 'cancelled'].includes(ticket.status)"
        />
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

    <el-row v-if="ticket" :gutter="16" class="mt-16">
      <el-col :xs="24" :md="8">
        <el-card shadow="never">
          <template #header><span class="card-title">基本信息</span></template>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="编号">{{ ticket.code }}</el-descriptions-item>
            <el-descriptions-item label="状态"><StatusTag :status="ticket.status" /></el-descriptions-item>
            <el-descriptions-item label="优先级"><PriorityTag :priority="ticket.priority" /></el-descriptions-item>
            <el-descriptions-item label="类型">{{ TICKET_TYPE_LABELS[ticket.type] }}</el-descriptions-item>
            <el-descriptions-item label="请求人">{{ ticket.requester_id }}</el-descriptions-item>
            <el-descriptions-item label="处理人">{{ ticket.assignee_id ?? '未指派' }}</el-descriptions-item>
            <el-descriptions-item label="创建时间">{{ formatDateTime(ticket.created_at) }}</el-descriptions-item>
            <el-descriptions-item label="解决时间">{{ formatDateTime(ticket.resolved_at) }}</el-descriptions-item>
            <el-descriptions-item label="累计挂起">{{ ticket.paused_minutes }} 分钟</el-descriptions-item>
            <el-descriptions-item label="描述">
              <pre class="pre-wrap">{{ ticket.description || '-' }}</pre>
            </el-descriptions-item>
            <el-descriptions-item v-if="ticket.solution" label="解决方案">
              <pre class="pre-wrap">{{ ticket.solution }}</pre>
            </el-descriptions-item>
          </el-descriptions>
        </el-card>

        <el-card v-if="canRate" shadow="never" class="mt-16">
          <template #header><span class="card-title">满意度评价</span></template>
          <el-rate v-model="ratingForm.rating" :max="5" show-score />
          <el-input v-model="ratingForm.comment" type="textarea" :rows="2" placeholder="补充评价（可选）" class="mt-8" />
          <el-button type="primary" class="mt-8" :disabled="!ratingForm.rating" @click="submitRating">
            提交评价
          </el-button>
        </el-card>
        <el-card v-else-if="ticket.rating" shadow="never" class="mt-16">
          <template #header><span class="card-title">满意度评价</span></template>
          <el-rate :model-value="ticket.rating" disabled show-score />
          <div class="text-muted mt-8">{{ ticket.rating_comment || '无文字评价' }}</div>
        </el-card>
      </el-col>

      <el-col :xs="24" :md="16">
        <el-card shadow="never">
          <template #header><span class="card-title">处理时间线</span></template>
          <Timeline :items="timelineItems" />
          <div class="comment-box">
            <el-input v-model="commentContent" type="textarea" :rows="2" placeholder="输入回复或内部备注" />
            <div class="comment-actions">
              <el-checkbox v-model="commentInternal">内部备注</el-checkbox>
              <el-button type="primary" size="small" :disabled="!commentContent" @click="submitComment">
                提交
              </el-button>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import StatusTag from '@/components/StatusTag.vue'
import PriorityTag from '@/components/PriorityTag.vue'
import SlaCountdown from '@/components/SlaCountdown.vue'
import Timeline from '@/components/Timeline.vue'
import UserSelect from '@/components/UserSelect.vue'
import { useTicketStore } from '@/stores/ticket'
import { useUserStore } from '@/stores/user'
import { fetchComments } from '@/api/platform'
import { useAuditTimeline } from '@/composables/useAuditTimeline'
import { formatDateTime, sortTimeline, type ActionDef, type TimelineItem } from '@/types/common'
import {
  ticketActions,
  TICKET_ACTION_LABELS,
  TICKET_TYPE_LABELS,
  type TicketAction,
  type TicketStatus
} from '@/types/ticket'

const route = useRoute()
const router = useRouter()
const store = useTicketStore()
const userStore = useUserStore()

const ticketId = computed(() => Number(route.params.id))
const ticket = computed(() => store.current)
const loading = computed(() => store.loading)

const comments = ref<{ id: number; content: string; is_internal: boolean; created_at: string; author_id: number }[]>([])
const commentContent = ref('')
const commentInternal = ref(false)

const dialogVisible = ref(false)
const submitting = ref(false)
const currentAction = ref<ActionDef<TicketAction> | null>(null)
const transitionForm = reactive<{ assignee_id?: number; solution?: string; reason?: string }>({})

const ratingForm = reactive<{ rating: number; comment: string }>({ rating: 0, comment: '' })

/** 状态流转时间线（审计日志）：403 时优雅降级隐藏 */
const audit = useAuditTimeline({
  entityType: computed(() => 'ticket'),
  entityId: computed(() => ticketId.value || null),
  actionLabels: TICKET_ACTION_LABELS
})

const availableActions = computed<ActionDef<TicketAction>[]>(() =>
  ticket.value ? ticketActions(ticket.value.status as TicketStatus, userStore.role as string) : []
)

const canRate = computed(
  () =>
    userStore.role === 'requestor' &&
    Boolean(ticket.value) &&
    ['resolved', 'closed'].includes(ticket.value!.status) &&
    !ticket.value!.rating
)

const dialogNeeds = computed(() => {
  const action = currentAction.value?.action
  return {
    assignee: action === 'assign',
    solution: action === 'resolve',
    reason: action === 'pending' || action === 'cancel'
  }
})

const dialogTitle = computed(() => (currentAction.value ? `执行操作：${currentAction.value.label}` : '执行操作'))

const timelineItems = computed<TimelineItem[]>(() => {
  const items: TimelineItem[] = []
  if (ticket.value) {
    items.push({
      id: 'created',
      time: ticket.value.created_at,
      title: '工单创建',
      type: 'primary',
      content: `请求人 ID：${ticket.value.requester_id}`
    })
  }
  comments.value.forEach((item) => {
    items.push({
      id: `comment-${item.id}`,
      time: item.created_at,
      title: item.is_internal ? '内部备注' : '公开回复',
      type: item.is_internal ? 'warning' : 'info',
      content: item.content
    })
  })
  // 合并「状态流转」（审计日志）；无权限时 audit.items 为空，仅显示评论
  if (!audit.forbidden.value) {
    audit.items.value.forEach((item) => items.push(item))
  }
  return sortTimeline(items)
})

async function loadDetail(): Promise<void> {
  await store.fetchDetail(ticketId.value)
  await Promise.all([loadComments(), audit.reload()])
}

async function loadComments(): Promise<void> {
  try {
    comments.value = await fetchComments({ biz_type: 'ticket', biz_id: ticketId.value })
  } catch {
    comments.value = []
  }
}

function openDialog(action: ActionDef<TicketAction>): void {
  currentAction.value = action
  transitionForm.assignee_id = ticket.value?.assignee_id ?? undefined
  transitionForm.solution = ''
  transitionForm.reason = ''
  dialogVisible.value = true
}

async function submitTransition(): Promise<void> {
  if (!currentAction.value) return
  const action = currentAction.value.action
  if (dialogNeeds.value.assignee && !transitionForm.assignee_id) {
    ElMessage.warning('请填写指派对象')
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
    await store.transition(ticketId.value, {
      action,
      assignee_id: transitionForm.assignee_id,
      solution: transitionForm.solution,
      reason: transitionForm.reason
    })
    ElMessage.success('操作成功')
    dialogVisible.value = false
    await loadDetail()
  } catch {
    // 409/422 错误提示由拦截器展示后端 message
  } finally {
    submitting.value = false
  }
}

async function submitComment(): Promise<void> {
  if (!commentContent.value) return
  await store.addComment(ticketId.value, commentContent.value, commentInternal.value)
  commentContent.value = ''
  commentInternal.value = false
  ElMessage.success('已提交')
  await loadComments()
}

async function submitRating(): Promise<void> {
  await store.rate(ticketId.value, { rating: ratingForm.rating, comment: ratingForm.comment })
  ElMessage.success('感谢评价')
  await loadDetail()
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
.comment-box {
  margin-top: 16px;
  border-top: 1px dashed #ebeef5;
  padding-top: 12px;
}
.comment-actions {
  margin-top: 8px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
