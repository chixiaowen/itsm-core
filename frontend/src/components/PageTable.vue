<template>
  <div class="page-table">
    <el-card v-if="$slots.filters" shadow="never" class="filter-card">
      <el-collapse>
        <el-collapse-item title="筛选条件" name="filters">
          <div class="filter-body">
            <slot name="filters" />
          </div>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <el-card shadow="never" class="table-card">
      <div v-if="$slots.toolbar" class="table-toolbar">
        <slot name="toolbar" />
      </div>
      <div class="table-body">
        <slot />
      </div>
      <div class="table-pager">
        <el-pagination
          background
          layout="total, sizes, prev, pager, next, jumper"
          :total="total"
          :current-page="page"
          :page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          @update:current-page="onPageChange"
          @update:page-size="onSizeChange"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    total: number
    page: number
    pageSize: number
    loading?: boolean
  }>(),
  { loading: false }
)

const emit = defineEmits<{
  (e: 'update:page', value: number): void
  (e: 'update:pageSize', value: number): void
}>()

function onPageChange(value: number): void {
  emit('update:page', value)
}

function onSizeChange(value: number): void {
  emit('update:pageSize', value)
}
</script>

<style scoped>
.page-table {
  padding: 16px;
}
.filter-card {
  margin-bottom: 12px;
}
.filter-body {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}
.table-card :deep(.el-card__body) {
  padding: 16px;
}
.table-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.table-pager {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}
</style>
