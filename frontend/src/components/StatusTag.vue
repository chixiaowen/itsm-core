<template>
  <el-tag :type="meta.type" :effect="effect" size="small" disable-transitions>{{ meta.label }}</el-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'

type TagType = 'primary' | 'success' | 'info' | 'warning' | 'danger'

interface StatusMeta {
  label: string
  type: TagType
}

const props = withDefaults(
  defineProps<{
    /** 六大模块 + CI/资产 + 风险等级 共用的状态值 */
    status: string
    effect?: 'dark' | 'light' | 'plain'
  }>(),
  { effect: 'light' }
)

/**
 * 状态色板（ARCHITECTURE §14.6 / PRD §7.2）：
 * 六大模块状态 + 资产/CI 状态 + 风险等级统一映射。
 */
const STATUS_META: Record<string, StatusMeta> = {
  // 通用
  draft: { label: '草稿', type: 'info' },
  new: { label: '新建', type: 'primary' },
  assigned: { label: '已指派', type: 'primary' },
  in_progress: { label: '处理中', type: 'warning' },
  pending: { label: '已挂起', type: 'info' },
  resolved: { label: '已解决', type: 'success' },
  closed: { label: '已关闭', type: 'info' },
  reopened: { label: '已重开', type: 'warning' },
  cancelled: { label: '已取消', type: 'info' },
  // 事件
  reported: { label: '已上报', type: 'primary' },
  triage: { label: '分诊中', type: 'warning' },
  escalated: { label: '已升级', type: 'danger' },
  // 问题
  investigating: { label: '调查中', type: 'warning' },
  known_error: { label: '已知错误', type: 'warning' },
  // 变更
  assessment: { label: '风险评估', type: 'primary' },
  pending_approval: { label: '待审批', type: 'warning' },
  approved: { label: '已批准', type: 'success' },
  rejected: { label: '已驳回', type: 'danger' },
  scheduled: { label: '已排期', type: 'primary' },
  implementing: { label: '实施中', type: 'warning' },
  implemented: { label: '已实施', type: 'success' },
  review: { label: '回顾中', type: 'warning' },
  rolled_back: { label: '已回滚', type: 'danger' },
  // 服务项
  published: { label: '已发布', type: 'success' },
  offline: { label: '已下线', type: 'info' },
  archived: { label: '已归档', type: 'info' },
  // CI / 资产
  planned: { label: '规划', type: 'info' },
  in_stock: { label: '在库', type: 'primary' },
  in_use: { label: '在用', type: 'success' },
  maintenance: { label: '维修中', type: 'warning' },
  retired: { label: '已退役', type: 'info' },
  disposed: { label: '已报废', type: 'info' },
  // 风险
  high: { label: '高', type: 'danger' },
  medium: { label: '中', type: 'warning' },
  low: { label: '低', type: 'primary' },
  // 用户
  active: { label: '启用', type: 'success' },
  disabled: { label: '禁用', type: 'info' }
}

const meta = computed<StatusMeta>(() => {
  const key = props.status || ''
  return STATUS_META[key] ?? { label: key || '未知', type: 'info' }
})
</script>
