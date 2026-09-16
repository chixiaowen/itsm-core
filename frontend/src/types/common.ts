/**
 * 通用类型与工具：与后端 §8 统一响应体、分页规范、角色/优先级枚举对齐。
 */

/** 七大角色（与 PRD §2.1 / ARCHITECTURE §8.5 一致） */
export type Role =
  | 'requestor'
  | 'agent'
  | 'resolver'
  | 'problem_manager'
  | 'change_manager'
  | 'cmdb_manager'
  | 'admin'

/** 优先级（PRD §5.2.1） */
export type Priority = 'P1' | 'P2' | 'P3' | 'P4'

/** SLA 三档状态（ARCHITECTURE §7 / PRD §5.2.1） */
export type SlaStatus = 'normal' | 'warning' | 'breached'

/** 统一响应体 {code, message, data} */
export interface ApiResponse<T> {
  code: number
  message: string
  data: T
}

/** 统一分页结构 {total, page, page_size, items} */
export interface PageResult<T> {
  total: number
  page: number
  page_size: number
  items: T[]
}

/** 统一分页查询参数 */
export interface PageQuery {
  page?: number
  page_size?: number
  sort_by?: string
  order?: 'asc' | 'desc'
}

/** 下拉/选项通用结构 */
export interface OptionItem {
  label: string
  value: string | number
}

/**
 * 状态机动作定义：详情页据此渲染「操作」按钮组。
 * 前端仅做可用性提示，后端仍为强校验方。
 */
export interface ActionDef<A extends string> {
  action: A
  label: string
  to: string
  roles: Role[]
  /** 危险操作（删除/归档/回滚）需二次确认 */
  danger?: boolean
}

/** 状态机流转映射：status -> 允许的 action 列表 */
export type ActionMap<S extends string, A extends string> = Partial<Record<S, ActionDef<A>[]>>

/** 角色显示名 */
export const ROLE_LABELS: Record<Role, string> = {
  requestor: '终端用户',
  agent: '服务台坐席',
  resolver: '二线工程师',
  problem_manager: '问题经理',
  change_manager: '变更经理',
  cmdb_manager: '配置管理员',
  admin: '系统管理员'
}

/** 优先级显示名 */
export const PRIORITY_LABELS: Record<Priority, string> = {
  P1: 'P1 紧急',
  P2: 'P2 高',
  P3: 'P3 中',
  P4: 'P4 低'
}

/** 优先级选项 */
export const PRIORITY_OPTIONS: OptionItem[] = [
  { label: 'P1 紧急', value: 'P1' },
  { label: 'P2 高', value: 'P2' },
  { label: 'P3 中', value: 'P3' },
  { label: 'P4 低', value: 'P4' }
]

/**
 * 从流转映射中筛选「当前状态 + 当前角色」允许的动作。
 * @param map 状态机流转映射
 * @param status 当前状态
 * @param role 当前用户角色
 */
export function actionsFor<S extends string, A extends string>(
  map: ActionMap<S, A>,
  status: string,
  role: Role | ''
): ActionDef<A>[] {
  if (!role) return []
  const list = (map as Record<string, ActionDef<A>[] | undefined>)[status] ?? []
  return list.filter((item) => item.roles.includes(role))
}

/** 时间格式化：RFC3339 -> 本地可读字符串 */
export function formatDateTime(value?: string | null): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const pad = (n: number): string => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(
    date.getHours()
  )}:${pad(date.getMinutes())}`
}

/** 仅日期 */
export function formatDate(value?: string | null): string {
  const text = formatDateTime(value)
  return text === '-' ? text : text.slice(0, 10)
}

/** 通用时间线条目（Timeline.vue 消费） */
export interface TimelineItem {
  id: string | number
  time: string
  title: string
  content?: string
  /** 语义类型，用于颜色：primary/success/warning/danger/info */
  type?: 'primary' | 'success' | 'warning' | 'danger' | 'info'
  /** 状态流转：来自审计日志 from_status，存在则由 StatusTag 渲染「from → to」 */
  fromStatus?: string
  /** 状态流转：来自审计日志 to_status */
  toStatus?: string
  /** 操作人 ID（审计日志 actor_id） */
  actorId?: number
  /** 原始动作标识（审计日志 action），供调用方按动作过滤/分组 */
  action?: string
}

/**
 * 从状态机流转映射生成「动作 -> 中文名」字典，供审计时间线展示动作标题。
 * @param map 状态机流转映射（ActionMap）
 */
export function actionLabelMap<A extends string>(map: ActionMap<string, A>): Record<string, string> {
  const out: Record<string, string> = {}
  Object.values(map).forEach((list) => {
    ;(list ?? []).forEach((def) => {
      out[def.action] = def.label
    })
  })
  return out
}

/**
 * 时间线按时间升序排序（无效时间置末尾），返回新数组。
 * @param items 时间线条目列表
 */
export function sortTimeline(items: TimelineItem[]): TimelineItem[] {
  const ts = (value: string): number => {
    const t = new Date(value).getTime()
    return Number.isNaN(t) ? Number.POSITIVE_INFINITY : t
  }
  return [...items].sort((a, b) => ts(a.time) - ts(b.time))
}
