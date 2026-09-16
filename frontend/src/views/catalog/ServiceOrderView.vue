<template>
  <div class="page-container">
    <el-card shadow="never">
      <div class="portal-header">
        <span class="portal-title">服务目录</span>
        <el-input
          v-model="keyword"
          placeholder="搜索服务项"
          clearable
          style="width: 260px"
          @keyup.enter="loadItems"
        />
      </div>
      <el-radio-group v-model="activeCategory" class="mt-16" @change="loadItems">
        <el-radio-button :value="0">全部</el-radio-button>
        <el-radio-button v-for="cat in flatCategories" :key="cat.id" :value="cat.id">
          {{ cat.name }}
        </el-radio-button>
      </el-radio-group>
    </el-card>

    <el-row v-loading="store.loading" :gutter="16" class="mt-16">
      <el-col v-for="item in store.portalItems" :key="item.id" :xs="24" :sm="12" :md="8">
        <el-card shadow="hover" class="item-card">
          <div class="item-name">{{ item.name }}</div>
          <div class="item-desc">{{ item.description || '暂无简介' }}</div>
          <div class="item-meta">
            <PriorityTag v-if="item.default_priority" :priority="item.default_priority" />
            <el-tag v-if="item.requires_approval" type="warning" size="small">需审批</el-tag>
          </div>
          <div class="item-actions">
            <el-button type="primary" size="small" @click="openOrder(item)">申请</el-button>
            <el-button size="small" @click="router.push(`/catalog/items/${item.id}`)">详情</el-button>
          </div>
        </el-card>
      </el-col>
      <el-col v-if="!store.loading && !store.portalItems.length" :span="24">
        <el-empty description="暂无可申请的服务项" />
      </el-col>
    </el-row>

    <el-dialog v-model="orderDialog" :title="`申请服务：${currentItem?.name || ''}`" width="640px">
      <el-alert
        v-if="currentItem"
        :title="`${currentItem.name}｜${currentItem.description || '无描述'}`"
        type="info"
        :closable="false"
        class="mb-16"
      />
      <el-form label-width="100px">
        <el-form-item label="标题">
          <el-input v-model="orderTitle" placeholder="可留空，默认使用服务项名称" />
        </el-form-item>
      </el-form>
      <DynamicForm ref="dynamicFormRef" v-model="formData" :fields="currentFields" />
      <template #footer>
        <el-button @click="orderDialog = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitOrder">提交申请</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import DynamicForm from '@/components/DynamicForm.vue'
import PriorityTag from '@/components/PriorityTag.vue'
import { useCatalogStore } from '@/stores/catalog'
import { parseFormSchema, type ServiceCategoryNode, type ServiceItem } from '@/types/catalog'

const router = useRouter()
const store = useCatalogStore()

const keyword = ref('')
const activeCategory = ref<number>(0)
const orderDialog = ref(false)
const submitting = ref(false)
const orderTitle = ref('')
const currentItem = ref<ServiceItem | null>(null)
const formData = ref<Record<string, string | number | null>>({})
const dynamicFormRef = ref<InstanceType<typeof DynamicForm> | null>(null)

interface FlatCategory {
  id: number
  name: string
}

function flatten(nodes: ServiceCategoryNode[], acc: FlatCategory[] = []): FlatCategory[] {
  nodes.forEach((node) => {
    acc.push({ id: node.id, name: node.name })
    if (node.children?.length) flatten(node.children, acc)
  })
  return acc
}

const flatCategories = computed(() => flatten(store.portalCategories))

const currentFields = computed(() => parseFormSchema(currentItem.value?.form_schema))

async function loadItems(): Promise<void> {
  const params: { category_id?: number; keyword?: string } = {}
  if (activeCategory.value) params.category_id = activeCategory.value
  if (keyword.value) params.keyword = keyword.value
  await store.fetchPortalItems(params)
}

function openOrder(item: ServiceItem): void {
  currentItem.value = item
  orderTitle.value = ''
  formData.value = {}
  orderDialog.value = true
}

async function submitOrder(): Promise<void> {
  if (!currentItem.value) return
  const valid = await dynamicFormRef.value?.validate()
  if (valid === false) {
    ElMessage.warning('请完善必填字段')
    return
  }
  submitting.value = true
  try {
    const ticket = await store.order(currentItem.value.id, {
      form_data: formData.value,
      title: orderTitle.value || undefined
    })
    ElMessage.success(`申请已提交，工单号：${ticket.code}`)
    orderDialog.value = false
    await router.push(`/tickets/${ticket.id}`)
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  await store.fetchPortalCategories()
  await loadItems()
})
</script>

<style scoped>
.portal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.portal-title {
  font-size: 18px;
  font-weight: 700;
  color: #2b6cb0;
}
.item-card {
  margin-bottom: 16px;
}
.item-name {
  font-size: 16px;
  font-weight: 600;
}
.item-desc {
  color: #909399;
  font-size: 13px;
  min-height: 38px;
  margin: 8px 0;
}
.item-meta {
  display: flex;
  gap: 8px;
  align-items: center;
  min-height: 24px;
}
.item-actions {
  margin-top: 12px;
  display: flex;
  gap: 8px;
}
</style>
