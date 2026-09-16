<template>
  <div v-loading="loading" class="page-container">
    <el-page-header :content="asset?.asset_no || '资产详情'" @back="router.back()" />

    <el-card v-if="asset" shadow="never" class="mt-16">
      <div class="detail-header">
        <span class="detail-title">{{ asset.name }}</span>
        <StatusTag :status="asset.status" />
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
          <el-button size="small" @click="router.push(`/cmdb/assets/${asset.id}/edit`)">编辑</el-button>
          <el-button type="success" size="small" @click="bindDialog = true">
            {{ asset.ci_id ? '更换绑定 CI' : '绑定 CI' }}
          </el-button>
          <el-button v-if="asset.ci_id" type="warning" size="small" @click="unbind">解绑 CI</el-button>
        </div>
      </div>
    </el-card>

    <el-row v-if="asset" :gutter="16" class="mt-16">
      <el-col :xs="24" :md="10">
        <el-card shadow="never">
          <template #header><span class="card-title">资产信息</span></template>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="资产编号">{{ asset.asset_no }}</el-descriptions-item>
            <el-descriptions-item label="名称">{{ asset.name }}</el-descriptions-item>
            <el-descriptions-item label="类别">{{ asset.category }}</el-descriptions-item>
            <el-descriptions-item label="状态"><StatusTag :status="asset.status" /></el-descriptions-item>
            <el-descriptions-item label="使用人">{{ asset.user_id ?? '-' }}</el-descriptions-item>
            <el-descriptions-item label="位置">{{ asset.location || '-' }}</el-descriptions-item>
            <el-descriptions-item label="供应商">{{ asset.vendor || '-' }}</el-descriptions-item>
            <el-descriptions-item label="采购日期">{{ formatDate(asset.purchase_date) }}</el-descriptions-item>
            <el-descriptions-item label="保修到期">{{ formatDate(asset.warranty_end) }}</el-descriptions-item>
            <el-descriptions-item label="绑定 CI">{{ asset.ci_id ?? '未绑定' }}</el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>

      <el-col :xs="24" :md="14">
        <el-card shadow="never">
          <template #header><span class="card-title">生命周期历史</span></template>
          <Timeline :items="historyItems" />
        </el-card>
      </el-col>
    </el-row>

    <!-- 生命周期流转 -->
    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="500px">
      <el-form label-width="100px">
        <el-form-item v-if="dialogNeeds.userLocation" label="使用人">
          <UserSelect v-model="transitionForm.user_id" placeholder="请选择使用人" style="width: 100%" />
        </el-form-item>
        <el-form-item v-if="dialogNeeds.userLocation" label="位置">
          <el-input v-model="transitionForm.location" placeholder="部署位置" />
        </el-form-item>
        <el-form-item label="备注/原因">
          <el-input v-model="transitionForm.remark" type="textarea" :rows="3" placeholder="流转说明" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitTransition">确认</el-button>
      </template>
    </el-dialog>

    <!-- 绑定 CI -->
    <el-dialog v-model="bindDialog" title="绑定 CI" width="420px">
      <el-form label-width="90px">
        <el-form-item label="CI ID">
          <el-input-number v-model="bindCiId" :min="1" :controls="false" style="width: 100%" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="bindDialog = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitBind">确认绑定</el-button>
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
import UserSelect from '@/components/UserSelect.vue'
import { useAssetStore } from '@/stores/asset'
import { useUserStore } from '@/stores/user'
import { useAuditTimeline } from '@/composables/useAuditTimeline'
import { formatDate, sortTimeline, type ActionDef, type TimelineItem } from '@/types/common'
import {
  ASSET_ACTION_LABELS,
  ASSET_STATUS_LABELS,
  assetActions,
  type AssetAction,
  type AssetStatus
} from '@/types/asset'

const route = useRoute()
const router = useRouter()
const store = useAssetStore()
const userStore = useUserStore()

const assetId = computed(() => Number(route.params.id))
const asset = computed(() => store.current)
const loading = computed(() => store.loading)

const dialogVisible = ref(false)
const bindDialog = ref(false)
const submitting = ref(false)
const bindCiId = ref<number>(1)
const currentAction = ref<ActionDef<AssetAction> | null>(null)

const transitionForm = reactive<{ remark?: string; user_id?: number; location?: string }>({})

const availableActions = computed<ActionDef<AssetAction>[]>(() =>
  asset.value ? assetActions(asset.value.status, userStore.role as string) : []
)

const dialogNeeds = computed(() => ({ userLocation: currentAction.value?.action === 'deploy' }))

const dialogTitle = computed(() => (currentAction.value ? `执行操作：${currentAction.value.label}` : '执行操作'))

/** 状态流转时间线（审计日志）：403 时优雅降级隐藏 */
const audit = useAuditTimeline({
  entityType: computed(() => 'asset'),
  entityId: computed(() => assetId.value || null),
  actionLabels: ASSET_ACTION_LABELS
})

/** 生命周期时间线：本地生命周期历史 + 审计记录（排除与历史重复的 transition 动作） */
const historyItems = computed<TimelineItem[]>(() => {
  const items: TimelineItem[] = store.history.map((item) => ({
    id: `history-${item.id}`,
    time: item.created_at,
    title: ASSET_STATUS_LABELS[item.to_status as AssetStatus] ?? '状态流转',
    fromStatus: item.from_status || undefined,
    toStatus: item.to_status || undefined,
    type: 'primary',
    content: item.remark
  }))
  if (!audit.forbidden.value) {
    // 审计中的 transition 与生命周期历史重复，过滤后仅保留 create/update/bind_ci 等操作
    audit.items.value
      .filter((item) => item.action !== 'transition')
      .forEach((item) => items.push(item))
  }
  return sortTimeline(items)
})

function openDialog(action: ActionDef<AssetAction>): void {
  currentAction.value = action
  transitionForm.remark = ''
  transitionForm.user_id = asset.value?.user_id ?? undefined
  transitionForm.location = asset.value?.location ?? undefined
  dialogVisible.value = true
}

async function submitTransition(): Promise<void> {
  if (!currentAction.value) return
  submitting.value = true
  try {
    await store.transition(assetId.value, {
      action: currentAction.value.action,
      remark: transitionForm.remark,
      user_id: transitionForm.user_id,
      location: transitionForm.location
    })
    ElMessage.success('操作成功')
    dialogVisible.value = false
    await loadAll()
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

async function submitBind(): Promise<void> {
  submitting.value = true
  try {
    await store.bindCi(assetId.value, bindCiId.value)
    ElMessage.success('已绑定 CI')
    bindDialog.value = false
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

async function unbind(): Promise<void> {
  try {
    await store.unbindCi(assetId.value)
    ElMessage.success('已解绑')
  } catch {
    /* handled by interceptor */
  }
}

async function loadAll(): Promise<void> {
  await store.fetchDetail(assetId.value)
  await Promise.all([
    store.fetchHistory(assetId.value).catch(() => undefined),
    audit.reload()
  ])
}

onMounted(() => {
  void loadAll()
})
</script>

<style scoped>
.header-actions {
  margin-left: auto;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
</style>
