<template>
  <el-tag :type="meta.type" effect="dark" size="small" disable-transitions>{{ meta.label }}</el-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { PRIORITY_LABELS, type Priority } from '@/types/common'

type TagType = 'primary' | 'success' | 'info' | 'warning' | 'danger'

const props = defineProps<{ priority: string }>()

/** 优先级色板：P1 红 / P2 橙 / P3 蓝 / P4 灰（PRD §7.2） */
const PRIORITY_TYPES: Record<Priority, TagType> = {
  P1: 'danger',
  P2: 'warning',
  P3: 'primary',
  P4: 'info'
}

const meta = computed(() => {
  const key = (props.priority || '') as Priority
  const type = PRIORITY_TYPES[key] ?? 'info'
  const label = PRIORITY_LABELS[key] ?? (props.priority || '未定级')
  return { type, label }
})
</script>
