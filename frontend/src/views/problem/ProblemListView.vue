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
        <el-option v-for="opt in PROBLEM_STATUS_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <UserSelect v-model="filters.assignee_id" placeholder="负责人" style="width: 200px" />
      <el-select v-model="filters.known_error" placeholder="已知错误" clearable style="width: 130px">
        <el-option label="仅已知错误" :value="true" />
        <el-option label="非已知错误" :value="false" />
      </el-select>
      <el-button type="primary" @click="search">查询</el-button>
      <el-button @click="reset">重置</el-button>
    </template>

    <template #toolbar>
      <span class="text-muted">共 {{ store.total }} 条问题</span>
      <el-button type="primary" @click="router.push('/problems/new')">新建问题</el-button>
    </template>

    <el-table v-loading="store.loading" :data="store.list" style="width: 100%" @row-click="goDetail">
      <el-table-column prop="code" label="编号" width="180" fixed />
      <el-table-column prop="title" label="标题" min-width="220" show-overflow-tooltip />
      <el-table-column label="状态" width="120">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column label="来源" width="110">
        <template #default="{ row }">{{ PROBLEM_SOURCE_LABELS[row.source as ProblemSource] }}</template>
      </el-table-column>
      <el-table-column label="关联事件数" width="110">
        <template #default="{ row }">{{ row.incident_ids?.length ?? 0 }}</template>
      </el-table-column>
      <el-table-column prop="assignee_id" label="负责人" width="100">
        <template #default="{ row }">{{ row.assignee_id ?? '-' }}</template>
      </el-table-column>
      <el-table-column label="创建时间" width="160">
        <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="120" fixed="right">
        <template #default="{ row }">
          <el-button text type="primary" @click.stop="goDetail(row as Problem)">详情</el-button>
        </template>
      </el-table-column>
      <template #empty><el-empty description="暂无问题" /></template>
    </el-table>
  </PageTable>
</template>

<script setup lang="ts">
import { onMounted, reactive } from 'vue'
import { useRouter } from 'vue-router'
import PageTable from '@/components/PageTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import UserSelect from '@/components/UserSelect.vue'
import { useProblemStore } from '@/stores/problem'
import { formatDateTime } from '@/types/common'
import {
  PROBLEM_SOURCE_LABELS,
  PROBLEM_STATUS_OPTIONS,
  type Problem,
  type ProblemQuery,
  type ProblemSource
} from '@/types/problem'

const router = useRouter()
const store = useProblemStore()

const filters = reactive<{
  status: string
  assignee_id: number | ''
  known_error: boolean | ''
}>({ status: '', assignee_id: '', known_error: '' })

function load(extra?: Partial<ProblemQuery>): void {
  const query: Partial<ProblemQuery> = { ...extra }
  if (filters.status) query.status = filters.status as ProblemQuery['status']
  if (typeof filters.assignee_id === 'number') query.assignee_id = filters.assignee_id
  if (filters.known_error !== '') query.known_error = filters.known_error
  void store.fetchList(query)
}

function search(): void {
  load({ page: 1 })
}

function reset(): void {
  filters.status = ''
  filters.assignee_id = ''
  filters.known_error = ''
  load({ page: 1 })
}

function goDetail(row: Problem): void {
  if (row?.id) void router.push(`/problems/${row.id}`)
}

onMounted(() => {
  load({ page: 1 })
})
</script>
