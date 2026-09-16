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
        <el-option v-for="opt in INCIDENT_STATUS_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-select v-model="filters.priority" placeholder="优先级" clearable style="width: 130px">
        <el-option v-for="opt in PRIORITY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-select v-model="filters.impact" placeholder="影响度" clearable style="width: 120px">
        <el-option v-for="opt in IMPACT_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-select v-model="filters.urgency" placeholder="紧急度" clearable style="width: 120px">
        <el-option v-for="opt in URGENCY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-button type="primary" @click="search">查询</el-button>
      <el-button @click="reset">重置</el-button>
    </template>

    <template #toolbar>
      <span class="text-muted">共 {{ store.total }} 条事件</span>
      <el-button type="primary" @click="router.push('/incidents/new')">上报事件</el-button>
    </template>

    <el-table v-loading="store.loading" :data="store.list" style="width: 100%" @row-click="goDetail">
      <el-table-column prop="code" label="编号" width="180" fixed />
      <el-table-column prop="title" label="标题" min-width="200" show-overflow-tooltip />
      <el-table-column label="影响度" width="90">
        <template #default="{ row }">{{ IMPACT_LABELS[row.impact as ImpactLevel] }}</template>
      </el-table-column>
      <el-table-column label="紧急度" width="90">
        <template #default="{ row }">{{ URGENCY_LABELS[row.urgency as UrgencyLevel] }}</template>
      </el-table-column>
      <el-table-column label="优先级" width="100">
        <template #default="{ row }"><PriorityTag :priority="row.priority" /></template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column label="升级级别" width="100">
        <template #default="{ row }">L{{ row.escalation_level }}</template>
      </el-table-column>
      <el-table-column prop="assignee_id" label="处理人" width="90">
        <template #default="{ row }">{{ row.assignee_id ?? '-' }}</template>
      </el-table-column>
      <el-table-column label="创建时间" width="160">
        <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="120" fixed="right">
        <template #default="{ row }">
          <el-button text type="primary" @click.stop="goDetail(row as Incident)">详情</el-button>
        </template>
      </el-table-column>
      <template #empty><el-empty description="暂无事件" /></template>
    </el-table>
  </PageTable>
</template>

<script setup lang="ts">
import { onMounted, reactive } from 'vue'
import { useRouter } from 'vue-router'
import PageTable from '@/components/PageTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import PriorityTag from '@/components/PriorityTag.vue'
import { useIncidentStore } from '@/stores/incident'
import { formatDateTime, PRIORITY_OPTIONS, type Priority } from '@/types/common'
import {
  IMPACT_LABELS,
  IMPACT_OPTIONS,
  INCIDENT_STATUS_OPTIONS,
  URGENCY_LABELS,
  URGENCY_OPTIONS,
  type ImpactLevel,
  type Incident,
  type IncidentQuery,
  type IncidentStatus,
  type UrgencyLevel
} from '@/types/incident'

const router = useRouter()
const store = useIncidentStore()

const filters = reactive({
  status: '',
  priority: '',
  impact: '',
  urgency: ''
})

function load(extra?: Partial<IncidentQuery>): void {
  const query: Partial<IncidentQuery> = { ...extra }
  if (filters.status) query.status = filters.status as IncidentStatus
  if (filters.priority) query.priority = filters.priority as Priority
  if (filters.impact) query.impact = filters.impact as ImpactLevel
  if (filters.urgency) query.urgency = filters.urgency as UrgencyLevel
  void store.fetchList(query)
}

function search(): void {
  load({ page: 1 })
}

function reset(): void {
  filters.status = ''
  filters.priority = ''
  filters.impact = ''
  filters.urgency = ''
  load({ page: 1 })
}

function goDetail(row: Incident): void {
  if (row?.id) void router.push(`/incidents/${row.id}`)
}

onMounted(() => {
  load({ page: 1 })
})
</script>
