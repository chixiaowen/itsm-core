<template>
  <PageTable
    :total="total"
    :page="query.page"
    :page-size="query.page_size"
    :loading="loading"
    @update:page="(v: number) => load({ page: v })"
    @update:page-size="(v: number) => load({ page: 1, page_size: v })"
  >
    <template #filters>
      <el-input v-model="filters.biz_type" placeholder="业务类型(ticket/incident/...)" clearable style="width: 220px" />
      <el-input v-model="filters.action" placeholder="动作(如 assign/close)" clearable style="width: 200px" />
      <el-input-number v-model="filters.actor_id" :min="1" :controls="false" placeholder="操作人 ID" style="width: 140px" />
      <el-date-picker
        v-model="dateRange"
        type="datetimerange"
        range-separator="至"
        start-placeholder="开始"
        end-placeholder="结束"
        style="width: 380px"
      />
      <el-button type="primary" @click="search">查询</el-button>
      <el-button @click="reset">重置</el-button>
    </template>

    <el-table v-loading="loading" :data="logs" style="width: 100%">
      <el-table-column prop="id" label="ID" width="90" />
      <el-table-column prop="actor_id" label="操作人" width="90" />
      <el-table-column prop="action" label="动作" width="140" />
      <el-table-column label="对象" width="160">
        <template #default="{ row }">{{ row.biz_type }} #{{ row.biz_id }}</template>
      </el-table-column>
      <el-table-column label="状态变化" width="200">
        <template #default="{ row }">{{ row.from_status || '—' }} → {{ row.to_status || '—' }}</template>
      </el-table-column>
      <el-table-column prop="client_ip" label="IP" width="140" />
      <el-table-column label="时间" width="170">
        <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
      </el-table-column>
      <template #empty><el-empty description="暂无审计日志" /></template>
    </el-table>
  </PageTable>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import PageTable from '@/components/PageTable.vue'
import { fetchAuditLogs } from '@/api/platform'
import { formatDateTime } from '@/types/common'
import type { AuditLog } from '@/types/platform'

const logs = ref<AuditLog[]>([])
const total = ref(0)
const loading = ref(false)
const query = reactive({ page: 1, page_size: 20 })
const filters = reactive<{ biz_type: string; action: string; actor_id?: number }>({
  biz_type: '',
  action: '',
  actor_id: undefined
})
const dateRange = ref<[string, string] | null>(null)

async function load(extra?: Partial<typeof query>): Promise<void> {
  if (extra) Object.assign(query, extra)
  loading.value = true
  try {
    const res = await fetchAuditLogs({
      page: query.page,
      page_size: query.page_size,
      biz_type: filters.biz_type || undefined,
      action: filters.action || undefined,
      actor_id: filters.actor_id || undefined,
      from: dateRange.value?.[0],
      to: dateRange.value?.[1]
    })
    logs.value = res.items
    total.value = res.total
  } finally {
    loading.value = false
  }
}

function search(): void {
  load({ page: 1 })
}

function reset(): void {
  filters.biz_type = ''
  filters.action = ''
  filters.actor_id = undefined
  dateRange.value = null
  load({ page: 1 })
}

onMounted(() => {
  void load()
})
</script>
