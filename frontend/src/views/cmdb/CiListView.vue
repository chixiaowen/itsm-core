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
      <el-select v-model="filters.ci_type" placeholder="CI 类型" clearable style="width: 150px">
        <el-option v-for="opt in CI_TYPE_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-select v-model="filters.status" placeholder="状态" clearable style="width: 140px">
        <el-option v-for="opt in CI_STATUS_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-input v-model="filters.keyword" placeholder="编码/名称关键字" clearable style="width: 200px" @keyup.enter="search" />
      <el-button type="primary" @click="search">查询</el-button>
      <el-button @click="reset">重置</el-button>
    </template>

    <template #toolbar>
      <span class="text-muted">共 {{ store.total }} 个 CI</span>
      <el-button type="primary" @click="router.push('/cmdb/cis/new')">新建 CI</el-button>
    </template>

    <el-table v-loading="store.loading" :data="store.cis" style="width: 100%" @row-click="(row) => goDetail(row as CI)">
      <el-table-column prop="code" label="编码" width="180" fixed />
      <el-table-column prop="name" label="名称" min-width="180" show-overflow-tooltip />
      <el-table-column label="类型" width="120">
        <template #default="{ row }">{{ CI_TYPE_LABELS[row.ci_type as CiType] }}</template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column prop="owner_id" label="负责人" width="100">
        <template #default="{ row }">{{ row.owner_id ?? '-' }}</template>
      </el-table-column>
      <el-table-column prop="relation_count" label="关系数" width="90">
        <template #default="{ row }">{{ row.relation_count ?? '-' }}</template>
      </el-table-column>
      <el-table-column label="操作" width="200" fixed="right">
        <template #default="{ row }">
          <el-button text type="primary" @click.stop="goDetail(row as CI)">详情</el-button>
          <el-button text type="primary" @click.stop="router.push(`/cmdb/cis/${row.id}/edit`)">编辑</el-button>
          <el-button text type="success" @click.stop="router.push(`/cmdb/cis/${row.id}/topology`)">拓扑</el-button>
        </template>
      </el-table-column>
      <template #empty><el-empty description="暂无 CI 数据" /></template>
    </el-table>
  </PageTable>
</template>

<script setup lang="ts">
import { onMounted, reactive } from 'vue'
import { useRouter } from 'vue-router'
import PageTable from '@/components/PageTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import { useCmdbStore } from '@/stores/cmdb'
import {
  CI_STATUS_OPTIONS,
  CI_TYPE_LABELS,
  CI_TYPE_OPTIONS,
  type CI,
  type CiQuery,
  type CiType
} from '@/types/cmdb'

const router = useRouter()
const store = useCmdbStore()

const filters = reactive({ ci_type: '', status: '', keyword: '' })

function load(extra?: Partial<CiQuery>): void {
  void store.fetchCis({
    ci_type: filters.ci_type as CiQuery['ci_type'],
    status: filters.status as CiQuery['status'],
    keyword: filters.keyword,
    ...extra
  })
}

function search(): void {
  load({ page: 1 })
}

function reset(): void {
  filters.ci_type = ''
  filters.status = ''
  filters.keyword = ''
  load({ page: 1 })
}

function goDetail(row: CI): void {
  if (row?.id) void router.push(`/cmdb/cis/${row.id}`)
}

onMounted(() => {
  load({ page: 1 })
})
</script>
