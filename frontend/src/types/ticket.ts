/**
 * 工单域类型 + 状态机流转映射（PRD §5.2 / ARCHITECTURE §6.3.1）。
 */
import type { ActionDef, ActionMap, PageQuery, Priority } from './common'
import { actionLabelMap } from './common'

export type TicketStatus =
  | 'draft'
  | 'new'
  | 'assigned'
  | 'in_progress'
  | 'pending'
  | 'resolved'
  | 'closed'
  | 'reopened'
  | 'cancelled'

export type TicketAction =
  | 'submit'
  | 'cancel'
  | 'assign'
  | 'start'
  | 'return'
  | 'pending'
  | 'resume'
  | 'resolve'
  | 'close'
  | 'reopen'

export type TicketType = 'manual' | 'service' | 'incident'

export interface Ticket {
  id: number
  code: string
  title: string
  description: string
  status: TicketStatus
  priority: Priority
  type: TicketType
  category_id: number | null
  requester_id: number
  assignee_id: number | null
  service_item_id: number | null
  source_incident_id: number | null
  sla_policy_id: number | null
  form_data: string
  solution: string
  paused_minutes: number
  paused_at: string | null
  first_responded_at: string | null
  response_due_at: string | null
  resolve_due_at: string | null
  resolved_at: string | null
  closed_at: string | null
  reopened_at: string | null
  rating: number | null
  rating_comment: string
  rated_at: string | null
  created_at: string
  updated_at: string
  sla_status: string
}

export interface TicketCategory {
  id: number
  name: string
  parent_id: number | null
  sort_order: number
  created_at: string
  updated_at: string
}

/** 分类树节点 */
export interface TicketCategoryNode extends TicketCategory {
  children: TicketCategoryNode[]
}

export interface TicketQuery extends PageQuery {
  status?: TicketStatus | ''
  priority?: Priority | ''
  category_id?: number | ''
  assignee_id?: number | ''
  sla_status?: string
  from?: string
  to?: string
  keyword?: string
}

export interface CreateTicketReq {
  title: string
  description?: string
  category_id?: number | null
  priority?: Priority
  requester_id?: number
  source?: string
}

export interface UpdateTicketReq {
  title?: string
  description?: string
  category_id?: number | null
  priority?: Priority
}

export interface TransitionReq {
  action: TicketAction
  solution?: string
  reason?: string
  assignee_id?: number
  priority?: Priority
}

export interface RatingReq {
  rating: number
  comment?: string
}

/** 状态机流转表：详情页据此渲染可变操作 */
export const TICKET_TRANSITIONS: ActionMap<TicketStatus, TicketAction> = {
  draft: [
    { action: 'submit', label: '提交', to: 'new', roles: ['agent', 'admin'] },
    { action: 'cancel', label: '取消', to: 'cancelled', roles: ['agent', 'admin'], danger: true }
  ],
  new: [
    { action: 'assign', label: '指派', to: 'assigned', roles: ['agent', 'admin'] },
    { action: 'cancel', label: '撤销', to: 'cancelled', roles: ['requestor'], danger: true }
  ],
  assigned: [
    { action: 'start', label: '开始处理', to: 'in_progress', roles: ['agent', 'resolver', 'admin'] },
    { action: 'return', label: '退回未指派', to: 'new', roles: ['agent', 'resolver', 'admin'] }
  ],
  in_progress: [
    { action: 'pending', label: '挂起', to: 'pending', roles: ['agent', 'resolver', 'admin'] },
    { action: 'resolve', label: '标记解决', to: 'resolved', roles: ['agent', 'resolver', 'admin'] }
  ],
  pending: [
    { action: 'resume', label: '恢复处理', to: 'in_progress', roles: ['agent', 'resolver', 'admin'] },
    { action: 'resolve', label: '直接解决', to: 'resolved', roles: ['agent', 'resolver', 'admin'] }
  ],
  resolved: [
    { action: 'close', label: '关闭', to: 'closed', roles: ['requestor', 'agent', 'admin'] },
    { action: 'reopen', label: '重开', to: 'reopened', roles: ['requestor'] }
  ],
  reopened: [
    { action: 'assign', label: '重新指派', to: 'assigned', roles: ['agent', 'admin'] },
    { action: 'start', label: '继续处理', to: 'in_progress', roles: ['agent', 'resolver', 'admin'] }
  ]
  // closed / cancelled 无表项 -> 全部拒绝
}

/** 状态显示名 */
export const TICKET_STATUS_LABELS: Record<TicketStatus, string> = {
  draft: '草稿',
  new: '新建',
  assigned: '已指派',
  in_progress: '处理中',
  pending: '已挂起',
  resolved: '已解决',
  closed: '已关闭',
  reopened: '已重开',
  cancelled: '已取消'
}

export const TICKET_STATUS_OPTIONS = Object.keys(TICKET_STATUS_LABELS).map((key) => ({
  label: TICKET_STATUS_LABELS[key as TicketStatus],
  value: key
}))

export const TICKET_TYPE_LABELS: Record<TicketType, string> = {
  manual: '手动提单',
  service: '服务目录',
  incident: '事件转单'
}

/** 判断某动作是否为当前状态下的可用操作 */
export function ticketActions(status: string, role: string | ''): ActionDef<TicketAction>[] {
  const list = (TICKET_TRANSITIONS as Record<string, ActionDef<TicketAction>[] | undefined>)[status]
  if (!list || !role) return []
  return list.filter((item) => item.roles.includes(role as never))
}

/** 动作 -> 中文名（审计时间线标题本地化） */
export const TICKET_ACTION_LABELS = actionLabelMap<TicketAction>(TICKET_TRANSITIONS)
