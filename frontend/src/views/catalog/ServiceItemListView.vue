<template>
  <PageTable
    :total="store.total"
    :page="store.query.page ?? 1"
    :page-size="store.query.page_size ?? 20"
    :loading="store.loading"
    @update:page="(v: number) => load({ page: v })"
    @update:page-size="(v: number) => load({ page: 1, page_size: v })"
  >
    <template #filters>
      <el-select v-model="filters.status" placeholder="状态" clearable style="width: 150px">
        <el-option v-for="opt in SERVICE_ITEM_STATUS_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-select v-model="filters.category_id" placeholder="分类" clearable style="width: 180px">
        <el-option v-for="opt in categoryOptions" :key="opt.id" :label="opt.name" :value="opt.id" />
      </el-select>
      <el-button type="primary" @click="search">查询</el-button>
      <el-button @click="reset">重置</el-button>
    </template>

    <template #toolbar>
      <span class="text-muted">共 {{ store.total }} 个服务项</span>
      <el-button type="primary" @click="router.push('/admin/service-items/new')">新建服务项</el-button>
    </template>

    <el-table v-loading="store.loading" :data="store.items" style="width: 100%">
      <el-table-column prop="name" label="名称" min-width="180" fixed />
      <el-table-column label="分类" width="150">
        <template #default="{ row }">{{ categoryName(row.category_id) }}</template>
      </el-table-column>
      <el-table-column label="SLA 策略" width="120">
        <template #default="{ row }">{{ row.sla_policy_id ?? '-' }}</template>
      </el-table-column>
      <el-table-column label="需审批" width="90">
        <template #default="{ row }">
          <el-tag v-if="row.requires_approval" type="warning" size="small">是</el-tag>
          <el-tag v-else type="info" size="small">否</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="默认优先级" width="110">
        <template #default="{ row }"><PriorityTag :priority="row.default_priority" /></template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column label="操作" width="240" fixed="right">
        <template #default="{ row }">
          <el-button text type="primary" @click="router.push(`/admin/service-items/${row.id}`)">详情</el-button>
          <el-button text type="primary" @click="router.push(`/admin/service-items/${row.id}/edit`)">编辑</el-button>
          <el-button v-if="row.status === 'pending_approval'" text type="success" @click="publish(row as ServiceItem)">发布</el-button>
          <el-button v-if="row.status === 'published'" text type="warning" @click="offline(row as ServiceItem)">下线</el-button>
        </template>
      </el-table-column>
      <template #empty><el-empty description="暂无服务项" /></template>
    </el-table>
  </PageTable>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import PageTable from '@/components/PageTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import PriorityTag from '@/components/PriorityTag.vue'
import { useCatalogStore } from '@/stores/catalog'
import { SERVICE_ITEM_STATUS_OPTIONS, type ServiceCategoryNode, type ServiceItem, type ServiceItemQuery } from '@/types/catalog'

const router = useRouter()
const store = useCatalogStore()

const filters = reactive<{ status: string; category_id: number | '' }>({ status: '', category_id: '' })

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

const categoryOptions = computed(() => flatten(store.categories))

function categoryName(id: number): string {
  return categoryOptions.value.find((item) => item.id === id)?.name ?? String(id)
}

function load(extra?: Partial<ServiceItemQuery>): void {
  const query: Partial<ServiceItemQuery> = { ...extra }
  if (filters.status) query.status = filters.status as ServiceItemQuery['status']
  if (filters.category_id !== '') query.category_id = filters.category_id
  void store.fetchItems(query)
}

function search(): void {
  load({ page: 1 })
}

function reset(): void {
  filters.status = ''
  filters.category_id = ''
  load({ page: 1 })
}

async function publish(item: ServiceItem): Promise<void> {
  await store.publish(item.id)
  ElMessage.success('已发布')
  load()
}

async function offline(item: ServiceItem): Promise<void> {
  await store.offline(item.id)
  ElMessage.success('已下线')
  load()
}

onMounted(async () => {
  await store.fetchTree()
  load({ page: 1 })
})
</script>
