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
      <el-select v-model="filters.category" placeholder="类别" clearable style="width: 140px">
        <el-option v-for="opt in ASSET_CATEGORY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-select v-model="filters.status" placeholder="状态" clearable style="width: 140px">
        <el-option v-for="opt in ASSET_STATUS_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-date-picker
        v-model="filters.warranty_before"
        type="date"
        value-format="YYYY-MM-DD"
        placeholder="保修到期前"
        style="width: 180px"
      />
      <el-button type="primary" @click="search">查询</el-button>
      <el-button @click="reset">重置</el-button>
    </template>

    <template #toolbar>
      <span class="text-muted">共 {{ store.total }} 项资产</span>
      <el-button type="primary" @click="router.push('/cmdb/assets/new')">新建资产</el-button>
    </template>

    <el-table v-loading="store.loading" :data="store.list" style="width: 100%" @row-click="(row) => goDetail(row as Asset)">
      <el-table-column prop="asset_no" label="资产编号" width="180" fixed />
      <el-table-column prop="name" label="名称" min-width="180" show-overflow-tooltip />
      <el-table-column prop="category" label="类别" width="110" />
      <el-table-column label="状态" width="110">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column prop="user_id" label="使用人" width="100">
        <template #default="{ row }">{{ row.user_id ?? '-' }}</template>
      </el-table-column>
      <el-table-column label="采购日期" width="130">
        <template #default="{ row }">{{ formatDate(row.purchase_date) }}</template>
      </el-table-column>
      <el-table-column label="保修到期" width="130">
        <template #default="{ row }">{{ formatDate(row.warranty_end) }}</template>
      </el-table-column>
      <el-table-column prop="ci_id" label="绑定 CI" width="100">
        <template #default="{ row }">{{ row.ci_id ?? '-' }}</template>
      </el-table-column>
      <el-table-column label="操作" width="120" fixed="right">
        <template #default="{ row }">
          <el-button text type="primary" @click.stop="goDetail(row as Asset)">详情</el-button>
        </template>
      </el-table-column>
      <template #empty><el-empty description="暂无资产" /></template>
    </el-table>
  </PageTable>
</template>

<script setup lang="ts">
import { onMounted, reactive } from 'vue'
import { useRouter } from 'vue-router'
import PageTable from '@/components/PageTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import { useAssetStore } from '@/stores/asset'
import { formatDate } from '@/types/common'
import {
  ASSET_CATEGORY_OPTIONS,
  ASSET_STATUS_OPTIONS,
  type Asset,
  type AssetQuery
} from '@/types/asset'

const router = useRouter()
const store = useAssetStore()

const filters = reactive<{ category: string; status: string; warranty_before: string }>({
  category: '',
  status: '',
  warranty_before: ''
})

function load(extra?: Partial<AssetQuery>): void {
  void store.fetchList({
    category: filters.category || undefined,
    status: filters.status as AssetQuery['status'],
    warranty_before: filters.warranty_before || undefined,
    ...extra
  })
}

function search(): void {
  load({ page: 1 })
}

function reset(): void {
  filters.category = ''
  filters.status = ''
  filters.warranty_before = ''
  load({ page: 1 })
}

function goDetail(row: Asset): void {
  if (row?.id) void router.push(`/cmdb/assets/${row.id}`)
}

onMounted(() => {
  load({ page: 1 })
})
</script>
