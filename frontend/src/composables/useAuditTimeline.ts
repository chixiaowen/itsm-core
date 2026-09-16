/**
 * 详情页「状态流转时间线」组合式函数（PRD §7 / ARCHITECTURE §5.1）。
 *
 * 从 GET /audit-logs?entity_type=&entity_id= 拉取审计记录，映射为 TimelineItem：
 * - 含 from_status/to_status 的记录：状态流转项（由 StatusTag 渲染「from → to」）
 * - 无 from/to 的记录：普通操作项
 *
 * 优雅降级：接口返回 403（无权限）或其它错误时静默失败，items 置空、forbidden 置位，
 * 调用方据此隐藏「状态流转」区块（只保留评论/本地时间线），不弹错误、不阻断页面。
 */
import { ref, watch, type Ref } from 'vue'
import { fetchEntityAuditLogs } from '@/api/platform'
import type { TimelineItem } from '@/types/common'
import type { AuditLog } from '@/types/platform'

const PAGE_SIZE = 100

/** 通用动词审计动作 -> 中文名（asset/cmdb/catalog 等使用通用动词作为 action） */
const GENERIC_ACTION_LABELS: Record<string, string> = {
  create: '创建',
  update: '更新',
  delete: '删除',
  transition: '状态流转',
  bind_ci: '绑定 CI',
  unbind_ci: '解绑 CI',
  order: '下单',
  comment: '评论',
  attach: '附件'
}

/** 判断错误是否为 403（无权限），用于区分「无权限」与「其它异常」 */
function isForbidden(error: unknown): boolean {
  const status =
    (error as { response?: { status?: number } } | undefined)?.response?.status ??
    (error as { status?: number } | undefined)?.status
  return status === 403
}

/** 将审计记录映射为时间线条目 */
function toTimelineItem(log: AuditLog, actionLabels?: Record<string, string>): TimelineItem {
  const isTransition = Boolean(log.from_status || log.to_status)
  return {
    id: `audit-${log.id}`,
    time: log.created_at,
    title: actionLabels?.[log.action] ?? GENERIC_ACTION_LABELS[log.action] ?? log.action ?? '操作',
    type: isTransition ? 'primary' : 'info',
    fromStatus: log.from_status || undefined,
    toStatus: log.to_status || undefined,
    actorId: log.actor_id,
    action: log.action
  }
}

export interface UseAuditTimelineOptions {
  /** 实体类型：ticket / incident / problem / change / ci / asset ... */
  entityType: Ref<string | null | undefined>
  /** 实体主键 */
  entityId: Ref<number | null | undefined>
  /** 动作 -> 中文名（由 actionLabelMap 生成），用于标题本地化 */
  actionLabels?: Record<string, string>
}

export interface UseAuditTimelineResult {
  /** 状态流转 / 操作时间线条目 */
  items: Ref<TimelineItem[]>
  /** 是否因无权限被降级隐藏 */
  forbidden: Ref<boolean>
  loading: Ref<boolean>
  /** 手动重载（详情页加载后调用） */
  reload: () => Promise<void>
}

/**
 * 创建审计时间线。
 * @param options 实体类型、主键与动作字典
 */
export function useAuditTimeline(options: UseAuditTimelineOptions): UseAuditTimelineResult {
  const items = ref<TimelineItem[]>([])
  const forbidden = ref(false)
  const loading = ref(false)

  async function reload(): Promise<void> {
    const type = options.entityType.value
    const id = options.entityId.value
    if (!type || !id) {
      items.value = []
      forbidden.value = false
      return
    }
    loading.value = true
    try {
      const res = await fetchEntityAuditLogs({
        entity_type: type,
        entity_id: id,
        page: 1,
        page_size: PAGE_SIZE
      })
      items.value = (res.items ?? []).map((log) => toTimelineItem(log, options.actionLabels))
      forbidden.value = false
    } catch (error) {
      // 静默降级：无权限或接口异常 -> 清空流转项，标记 forbidden
      items.value = []
      forbidden.value = isForbidden(error)
    } finally {
      loading.value = false
    }
  }

  watch([options.entityType, options.entityId], () => {
    void reload()
  })

  return { items, forbidden, loading, reload }
}
