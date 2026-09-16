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
        <el-option v-for="opt in CHANGE_STATUS_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-select v-model="filters.change_type" placeholder="变更类型" clearable style="width: 140px">
        <el-option v-for="opt in CHANGE_TYPE_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-select v-model="filters.risk_level" placeholder="风险等级" clearable style="width: 130px">
        <el-option v-for="opt in RISK_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-button type="primary" @click="search">查询</el-button>
      <el-button @click="reset">重置</el-button>
    </template>

    <template #toolbar>
      <span class="text-muted">共 {{ store.total }} 条变更</span>
      <el-button type="primary" @click="router.push('/changes/new')">提交变更</el-button>
    </template>

    <el-table v-loading="store.loading" :data="store.list" style="width: 100%" @row-click="(row) => goDetail(row as Change)">
      <el-table-column prop="code" label="编号" width="180" fixed />
      <el-table-column prop="title" label="标题" min-width="200" show-overflow-tooltip />
      <el-table-column label="类型" width="110">
        <template #default="{ row }">{{ CHANGE_TYPE_LABELS[row.change_type as ChangeType] }}</template>
      </el-table-column>
      <el-table-column label="风险" width="90">
        <template #default="{ row }"><StatusTag :status="row.risk_level" /></template>
      </el-table-column>
      <el-table-column label="状态" width="120">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column label="窗口开始" width="160">
        <template #default="{ row }">{{ formatDateTime(row.window_start) }}</template>
      </el-table-column>
      <el-table-column prop="requester_id" label="申请人" width="100" />
      <el-table-column label="操作" width="120" fixed="right">
        <template #default="{ row }">
          <el-button text type="primary" @click.stop="goDetail(row as Change)">详情</el-button>
        </template>
      </el-table-column>
      <template #empty><el-empty description="暂无变更" /></template>
    </el-table>
  </PageTable>
</template>

<script setup lang="ts">
import { onMounted, reactive } from 'vue'
import { useRouter } from 'vue-router'
import PageTable from '@/components/PageTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import { useChangeStore } from '@/stores/change'
import { formatDateTime } from '@/types/common'
import {
  CHANGE_STATUS_OPTIONS,
  CHANGE_TYPE_LABELS,
  CHANGE_TYPE_OPTIONS,
  RISK_OPTIONS,
  type Change,
  type ChangeQuery,
  type ChangeType
} from '@/types/change'

const router = useRouter()
const store = useChangeStore()

const filters = reactive({ status: '', change_type: '', risk_level: '' })

function load(extra?: Partial<ChangeQuery>): void {
  void store.fetchList({
    status: filters.status as ChangeQuery['status'],
    change_type: filters.change_type as ChangeQuery['change_type'],
    risk_level: filters.risk_level as ChangeQuery['risk_level'],
    ...extra
  })
}

function search(): void {
  load({ page: 1 })
}

function reset(): void {
  filters.status = ''
  filters.change_type = ''
  filters.risk_level = ''
  load({ page: 1 })
}

function goDetail(row: Change): void {
  if (row?.id) void router.push(`/changes/${row.id}`)
}

onMounted(() => {
  load({ page: 1 })
})
</script>
