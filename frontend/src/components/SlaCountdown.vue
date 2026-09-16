<template>
  <span class="sla-countdown" :class="`sla-${statusClass}`">
    <span v-if="statusClass === 'warning'" class="sla-icon">⚠</span>
    {{ displayText }}
  </span>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps<{
  /** normal / warning / breached */
  slaStatus?: string
  /** 截止时间 RFC3339 */
  dueAt?: string | null
  /** 是否已到达终点（resolved/closed），到达后不再倒计时 */
  finished?: boolean
}>()

const now = ref<number>(Date.now())
let timer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  timer = setInterval(() => {
    now.value = Date.now()
  }, 1000)
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})

const diffMs = computed<number>(() => {
  if (!props.dueAt) return 0
  const due = new Date(props.dueAt).getTime()
  if (Number.isNaN(due)) return 0
  return due - now.value
})

const statusClass = computed<string>(() => {
  if (props.slaStatus === 'breached') return 'breached'
  if (props.slaStatus === 'warning') return 'warning'
  if (!props.slaStatus && diffMs.value < 0 && !props.finished) return 'breached'
  return 'normal'
})

function humanize(ms: number): string {
  const abs = Math.abs(ms)
  const totalMinutes = Math.floor(abs / 60000)
  const days = Math.floor(totalMinutes / 1440)
  const hours = Math.floor((totalMinutes % 1440) / 60)
  const minutes = totalMinutes % 60
  const parts: string[] = []
  if (days > 0) parts.push(`${days}天`)
  if (hours > 0) parts.push(`${hours}小时`)
  parts.push(`${minutes}分钟`)
  return parts.join('')
}

const displayText = computed<string>(() => {
  if (!props.dueAt) return '无 SLA'
  if (props.finished) return '已结束'
  const ms = diffMs.value
  if (ms < 0) return `已超期 ${humanize(ms)}`
  return `剩余 ${humanize(ms)}`
})
</script>

<style scoped>
.sla-countdown {
  font-size: 13px;
}
.sla-normal {
  color: #909399;
}
.sla-warning {
  color: #e6a23c;
}
.sla-breached {
  color: #f56c6c;
  font-weight: 700;
}
.sla-icon {
  margin-right: 2px;
}
</style>
