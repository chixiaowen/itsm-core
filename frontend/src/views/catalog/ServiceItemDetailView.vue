<template>
  <div v-loading="loading" class="page-container">
    <el-page-header :content="item?.name || '服务项详情'" @back="router.back()" />

    <el-card v-if="item" shadow="never" class="mt-16">
      <div class="detail-header">
        <span class="detail-title">{{ item.name }}</span>
        <StatusTag :status="item.status" />
        <PriorityTag v-if="item.default_priority" :priority="item.default_priority" />
        <el-tag v-if="item.requires_approval" type="warning" size="small">需审批</el-tag>
        <div class="header-actions">
          <el-button
            v-for="action in availableActions"
            :key="action.action"
            :type="action.danger ? 'danger' : 'primary'"
            size="small"
            @click="onAction(action)"
          >
            {{ action.label }}
          </el-button>
          <el-button v-if="isAdmin" size="small" @click="router.push(`/admin/service-items/${item.id}/edit`)">
            编辑
          </el-button>
        </div>
      </div>
    </el-card>

    <el-row v-if="item" :gutter="16" class="mt-16">
      <el-col :xs="24" :md="12">
        <el-card shadow="never">
          <template #header><span class="card-title">服务项信息</span></template>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="名称">{{ item.name }}</el-descriptions-item>
            <el-descriptions-item label="状态"><StatusTag :status="item.status" /></el-descriptions-item>
            <el-descriptions-item label="SLA 策略">{{ item.sla_policy_id ?? '默认' }}</el-descriptions-item>
            <el-descriptions-item label="默认优先级">{{ item.default_priority || 'P4' }}</el-descriptions-item>
            <el-descriptions-item label="需审批">{{ item.requires_approval ? '是' : '否' }}</el-descriptions-item>
            <el-descriptions-item label="描述">
              <pre class="pre-wrap">{{ item.description || '-' }}</pre>
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>

      <el-col :xs="24" :md="12">
        <el-card shadow="never">
          <template #header><span class="card-title">表单字段定义（form_schema）</span></template>
          <el-table :data="fields" size="small">
            <el-table-column prop="label" label="字段" />
            <el-table-column prop="name" label="字段名" />
            <el-table-column prop="type" label="类型" width="90" />
            <el-table-column label="必填" width="70">
              <template #default="{ row }">{{ row.required ? '是' : '否' }}</template>
            </el-table-column>
            <template #empty><el-empty description="未定义表单字段" /></template>
          </el-table>
        </el-card>

        <el-card v-if="canOrder" shadow="never" class="mt-16">
          <template #header><span class="card-title">申请该服务</span></template>
          <el-form label-width="90px">
            <el-form-item label="标题">
              <el-input v-model="orderTitle" placeholder="可留空" />
            </el-form-item>
          </el-form>
          <DynamicForm ref="dynamicFormRef" v-model="formData" :fields="fields" />
          <el-button type="primary" :loading="submitting" @click="submitOrder">提交申请</el-button>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import StatusTag from '@/components/StatusTag.vue'
import PriorityTag from '@/components/PriorityTag.vue'
import DynamicForm from '@/components/DynamicForm.vue'
import { useCatalogStore } from '@/stores/catalog'
import { useUserStore } from '@/stores/user'
import { parseFormSchema, serviceItemActions, type ServiceItemAction } from '@/types/catalog'
import type { ActionDef } from '@/types/common'

const route = useRoute()
const router = useRouter()
const store = useCatalogStore()
const userStore = useUserStore()

const itemId = computed(() => Number(route.params.id))
const item = computed(() => store.current)
const loading = computed(() => store.loading)
const isAdmin = computed(() => userStore.role === 'admin')

const fields = computed(() => parseFormSchema(item.value?.form_schema))
const canOrder = computed(() => item.value?.status === 'published')

const orderTitle = ref('')
const formData = ref<Record<string, string | number | null>>({})
const submitting = ref(false)
const dynamicFormRef = ref<InstanceType<typeof DynamicForm> | null>(null)

const availableActions = computed<ActionDef<ServiceItemAction>[]>(() =>
  item.value ? serviceItemActions(item.value.status, userStore.role as string) : []
)

async function onAction(action: ActionDef<ServiceItemAction>): Promise<void> {
  if (!item.value) return
  if (action.action === 'publish' || action.action === 'republish') {
    await store.publish(item.value.id)
    ElMessage.success('已发布')
  } else if (action.action === 'offline') {
    await store.offline(item.value.id)
    ElMessage.success('已下线')
  } else if (action.action === 'archive') {
    await store.archive(item.value.id)
    ElMessage.success('已归档')
  } else {
    ElMessage.info(`请通过状态流转接口执行：${action.label}`)
  }
  await store.fetchItem(itemId.value)
}

async function submitOrder(): Promise<void> {
  if (!item.value) return
  const valid = await dynamicFormRef.value?.validate()
  if (valid === false) {
    ElMessage.warning('请完善必填字段')
    return
  }
  submitting.value = true
  try {
    const ticket = await store.order(item.value.id, {
      form_data: formData.value,
      title: orderTitle.value || undefined
    })
    ElMessage.success(`申请已提交，工单号：${ticket.code}`)
    await router.push(`/tickets/${ticket.id}`)
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  void store.fetchItem(itemId.value)
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
