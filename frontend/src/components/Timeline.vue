<template>
  <el-timeline v-if="items.length">
    <el-timeline-item
      v-for="item in items"
      :key="item.id"
      :timestamp="formatDateTime(item.time)"
      :type="item.type || 'primary'"
    >
      <div class="timeline-title">{{ item.title }}</div>
      <div v-if="item.fromStatus || item.toStatus" class="timeline-flow">
        <StatusTag v-if="item.fromStatus" :status="item.fromStatus" />
        <span v-else class="timeline-dash">—</span>
        <span class="timeline-arrow">→</span>
        <StatusTag v-if="item.toStatus" :status="item.toStatus" />
        <span v-else class="timeline-dash">—</span>
      </div>
      <div v-if="item.content" class="timeline-content">{{ item.content }}</div>
    </el-timeline-item>
  </el-timeline>
  <el-empty v-else description="暂无时间线记录" />
</template>

<script setup lang="ts">
import { formatDateTime, type TimelineItem } from '@/types/common'
import StatusTag from '@/components/StatusTag.vue'

defineProps<{ items: TimelineItem[] }>()
</script>

<style scoped>
.timeline-title {
  font-weight: 600;
}
.timeline-flow {
  margin-top: 4px;
  display: flex;
  align-items: center;
  gap: 6px;
}
.timeline-arrow {
  color: #909399;
}
.timeline-dash {
  color: #c0c4cc;
}
.timeline-content {
  margin-top: 4px;
  color: #606266;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
