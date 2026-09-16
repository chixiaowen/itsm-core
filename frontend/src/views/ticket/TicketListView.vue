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
      <el-input
        v-model="filters.keyword"
        placeholder="编号 / 标题关键字"
        clearable
        style="width: 220px"
        @keyup.enter="search"
      />
      <el-select v-model="filters.status" placeholder="状态" clearable style="width: 150px">
        <el-option v-for="opt in TICKET_STATUS_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-select v-model="filters.priority" placeholder="优先级" clearable style="width: 130px">
        <el-option v-for="opt in PRIORITY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-select v-model="filters.sla_status" placeholder="SLA 状态" clearable style="width: 130px">
        <el-option label="正常" value="normal" />
        <el-option label="即将超期" value="warning" />
        <el-option label="已超期" value="breached" />
      </el-select>
      <el-button type="primary" @click="search">查询</el-button>
      <el-button @click="reset">重置</el-button>
    </template>

    <template #toolbar>
      <span class="text-muted">共 {{ store.total }} 条工单</span>
      <el-button type="primary" @click="router.push('/tickets/new')">新建工单</el-button>
    </template>

    <el-table
      v-loading="store.loading"
      :data="store.list"
      style="width: 100%"
      @row-click="goDetail"
    >
      <el-table-column prop="code" label="编号" width="180" fixed />
      <el-table-column prop="title" label="标题" min-width="220" show-overflow-tooltip />
      <el-table-column label="优先级" width="100">
        <template #default="{ row }"><PriorityTag :priority="row.priority" /></template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column prop="requester_id" label="请求人" width="90" />
      <el-table-column prop="assignee_id" label="处理人" width="90">
        <template #default="{ row }">{{ row.assignee_id ?? '-' }}</template>
      </el-table-column>
      <el-table-column label="SLA" width="180">
        <template #default="{ row }">
          <SlaCountdown
            :sla-status="row.sla_status"
            :due-at="row.resolve_due_at"
            :finished="['resolved', 'closed', 'cancelled'].includes(row.status)"
          />
        </template>
      </el-table-column>
      <el-table-column label="创建时间" width="160">
        <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="120" fixed="right">
        <template #default="{ row }">
          <el-button text type="primary" @click.stop="goDetail(row as Ticket)">详情</el-button>
        </template>
      </el-table-column>
      <template #empty><el-empty description="暂无工单" /></template>
    </el-table>
  </PageTable>
</template>

<script setup lang="ts">
import { onMounted, reactive } from 'vue'
import { useRouter } from 'vue-router'
import PageTable from '@/components/PageTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import PriorityTag from '@/components/PriorityTag.vue'
import SlaCountdown from '@/components/SlaCountdown.vue'
import { useTicketStore } from '@/stores/ticket'
import { formatDateTime, PRIORITY_OPTIONS, type Priority } from '@/types/common'
import { TICKET_STATUS_OPTIONS, type Ticket, type TicketQuery, type TicketStatus } from '@/types/ticket'

const router = useRouter()
const store = useTicketStore()

const filters = reactive({
  keyword: '',
  status: '',
  priority: '',
  sla_status: ''
})

function load(extra?: Partial<TicketQuery>): void {
  const query: Partial<TicketQuery> = { ...extra }
  if (filters.keyword) query.keyword = filters.keyword
  if (filters.status) query.status = filters.status as TicketStatus
  if (filters.priority) query.priority = filters.priority as Priority
  if (filters.sla_status) query.sla_status = filters.sla_status
  void store.fetchList(query)
}

function search(): void {
  load({ page: 1 })
}

function reset(): void {
  filters.keyword = ''
  filters.status = ''
  filters.priority = ''
  filters.sla_status = ''
  load({ page: 1 })
}

function goDetail(row: Ticket): void {
  if (row?.id) void router.push(`/tickets/${row.id}`)
}

onMounted(() => {
  load({ page: 1 })
})
</script>
