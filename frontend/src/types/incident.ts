/**
 * 事件域类型 + 优先级矩阵 + 状态机流转映射（PRD §5.3 / ARCHITECTURE §6.3.2）。
 */
import type { ActionDef, ActionMap, PageQuery, Priority } from './common'
import { actionLabelMap } from './common'

export type IncidentStatus =
  | 'reported'
  | 'triage'
  | 'in_progress'
  | 'escalated'
  | 'pending'
  | 'resolved'
  | 'closed'
  | 'cancelled'

export type IncidentAction =
  | 'triage'
  | 'cancel'
  | 'confirm'
  | 'false_positive'
  | 'escalate'
  | 'take_over'
  | 'pending'
  | 'resume'
  | 'resolve'
  | 'close'
  | 'revert'

export type ImpactLevel = 'high' | 'medium' | 'low'
export type UrgencyLevel = 'high' | 'medium' | 'low'
export type EscalationType = 'functional' | 'hierarchical'

export interface Incident {
  id: number
  code: string
  title: string
  description: string
  status: IncidentStatus
  impact: ImpactLevel
  urgency: UrgencyLevel
  priority: Priority
  priority_overridden: boolean
  escalation_level: number
  reporter_id: number
  assignee_id: number | null
  problem_id: number | null
  ticket_id: number | null
  sla_policy_id: number | null
  solution: string
  review_conclusion: string
  occurred_at: string | null
  resolved_at: string | null
  closed_at: string | null
  response_due_at: string | null
  resolve_due_at: string | null
  created_at: string
  updated_at: string
  sla_status: string
}

export interface IncidentEscalation {
  id: number
  incident_id: number
  level: number
  type: EscalationType
  reason: string
  from_assignee_id: number | null
  to_assignee_id: number | null
  actor_id: number
  created_at: string
}

export interface IncidentQuery extends PageQuery {
  status?: IncidentStatus | ''
  priority?: Priority | ''
  impact?: ImpactLevel | ''
  urgency?: UrgencyLevel | ''
  escalation_level?: number | ''
  from?: string
  to?: string
}

export interface CreateIncidentReq {
  title: string
  description?: string
  impact: ImpactLevel
  urgency: UrgencyLevel
  occurred_at?: string
  ci_ids?: number[]
}

export interface UpdateIncidentReq {
  title?: string
  description?: string
  impact?: ImpactLevel
  urgency?: UrgencyLevel
}

export interface IncidentTransitionReq {
  action: IncidentAction
  solution?: string
  reason?: string
  /** 确认（确认并指派）/ 接手时指定的处理人 */
  assignee_id?: number
}

export interface EscalateReq {
  type: EscalationType
  reason: string
  to_assignee_id?: number
}

export interface PriorityMatrixCell {
  impact: ImpactLevel
  urgency: UrgencyLevel
  priority: Priority
}

export const IMPACT_LABELS: Record<ImpactLevel, string> = {
  high: '高',
  medium: '中',
  low: '低'
}

export const URGENCY_LABELS: Record<UrgencyLevel, string> = {
  high: '高',
  medium: '中',
  low: '低'
}

export const IMPACT_OPTIONS = [
  { label: '高', value: 'high' },
  { label: '中', value: 'medium' },
  { label: '低', value: 'low' }
]

export const URGENCY_OPTIONS = [
  { label: '高', value: 'high' },
  { label: '中', value: 'medium' },
  { label: '低', value: 'low' }
]

export const ESCALATION_TYPE_LABELS: Record<EscalationType, string> = {
  functional: '功能升级',
  hierarchical: '层级升级'
}

/** 影响度 × 紧急度 → 优先级（本地预览，与后端矩阵一致） */
export function computePriority(impact: ImpactLevel, urgency: UrgencyLevel): Priority {
  const score: Record<ImpactLevel | UrgencyLevel, number> = { high: 3, medium: 2, low: 1 }
  const sum = score[impact] + score[urgency]
  if (sum >= 6) return 'P1'
  if (sum === 5) return 'P2'
  if (sum === 4) return 'P3'
  return 'P4'
}

export const INCIDENT_TRANSITIONS: ActionMap<IncidentStatus, IncidentAction> = {
  reported: [
    { action: 'triage', label: '分诊', to: 'triage', roles: ['agent', 'admin'] },
    { action: 'cancel', label: '撤销上报', to: 'cancelled', roles: ['requestor'], danger: true }
  ],
  triage: [
    { action: 'confirm', label: '确认并指派', to: 'in_progress', roles: ['agent', 'admin'] },
    { action: 'false_positive', label: '误报关闭', to: 'resolved', roles: ['agent', 'admin'] },
    { action: 'cancel', label: '判定非事件', to: 'cancelled', roles: ['agent', 'admin'], danger: true }
  ],
  in_progress: [
    {
      action: 'escalate',
      label: '升级',
      to: 'escalated',
      roles: ['agent', 'resolver', 'problem_manager', 'admin']
    },
    { action: 'pending', label: '挂起', to: 'pending', roles: ['agent', 'resolver', 'admin'] },
    { action: 'resolve', label: '解决', to: 'resolved', roles: ['agent', 'resolver', 'admin'] }
  ],
  escalated: [
    { action: 'take_over', label: '接手处理', to: 'in_progress', roles: ['agent', 'resolver', 'admin'] }
  ],
  pending: [
    { action: 'resume', label: '恢复', to: 'in_progress', roles: ['agent', 'resolver', 'admin'] },
    { action: 'resolve', label: '解决', to: 'resolved', roles: ['agent', 'resolver', 'admin'] }
  ],
  resolved: [
    { action: 'close', label: '关闭', to: 'closed', roles: ['agent', 'admin'] },
    { action: 'revert', label: '回退', to: 'in_progress', roles: ['agent', 'admin'] }
  ]
}

export const INCIDENT_STATUS_LABELS: Record<IncidentStatus, string> = {
  reported: '已上报',
  triage: '分诊中',
  in_progress: '处理中',
  escalated: '已升级',
  pending: '已挂起',
  resolved: '已解决',
  closed: '已关闭',
  cancelled: '已取消'
}

export const INCIDENT_STATUS_OPTIONS = Object.keys(INCIDENT_STATUS_LABELS).map((key) => ({
  label: INCIDENT_STATUS_LABELS[key as IncidentStatus],
  value: key
}))

export function incidentActions(status: string, role: string | ''): ActionDef<IncidentAction>[] {
  const list = (INCIDENT_TRANSITIONS as Record<string, ActionDef<IncidentAction>[] | undefined>)[status]
  if (!list || !role) return []
  return list.filter((item) => item.roles.includes(role as never))
}

/** 动作 -> 中文名（审计时间线标题本地化） */
export const INCIDENT_ACTION_LABELS = actionLabelMap<IncidentAction>(INCIDENT_TRANSITIONS)
