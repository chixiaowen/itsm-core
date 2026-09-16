/**
 * 变更域类型 + 状态机流转映射 + CAB 会签（PRD §5.5 / ARCHITECTURE §6.3.4、§6.4）。
 */
import type { ActionDef, ActionMap, PageQuery } from './common'
import { actionLabelMap } from './common'

export type ChangeStatus =
  | 'draft'
  | 'assessment'
  | 'pending_approval'
  | 'approved'
  | 'rejected'
  | 'scheduled'
  | 'implementing'
  | 'implemented'
  | 'review'
  | 'closed'
  | 'rolled_back'
  | 'cancelled'

export type ChangeAction =
  | 'submit_assessment'
  | 'pre_authorize'
  | 'cancel'
  | 'submit_approval'
  | 'approve'
  | 'reject'
  | 'revise'
  | 'schedule'
  | 'start_implement'
  | 'complete'
  | 'rollback'
  | 'review'
  | 'close'
  | 'resubmit'

export type ChangeType = 'standard' | 'normal' | 'emergency'
export type RiskLevel = 'high' | 'medium' | 'low'
export type ApprovalDecision = 'approved' | 'rejected'

export interface Change {
  id: number
  code: string
  title: string
  description: string
  change_type: ChangeType
  status: ChangeStatus
  risk_level: RiskLevel
  impact_analysis: string
  plan: string
  rollback_plan: string
  implement_result: string
  rollback_reason: string
  review_conclusion: string
  requester_id: number
  manager_id: number | null
  pre_authorized: boolean
  window_start: string | null
  window_end: string | null
  closed_at: string | null
  created_at: string
  updated_at: string
}

export interface ChangeApproval {
  id: number
  change_id: number
  approver_id: number
  decision: ApprovalDecision | ''
  comment: string
  decided_at: string | null
  created_at: string
}

export interface ChangeQuery extends PageQuery {
  status?: ChangeStatus | ''
  change_type?: ChangeType | ''
  risk_level?: RiskLevel | ''
  manager_id?: number | ''
  window_from?: string
  window_to?: string
}

export interface CreateChangeReq {
  title: string
  description?: string
  change_type: ChangeType
  risk_level: RiskLevel
}

export interface UpdateChangeReq {
  title?: string
  description?: string
  change_type?: ChangeType
  risk_level?: RiskLevel
  impact_analysis?: string
  plan?: string
  rollback_plan?: string
  window_start?: string | null
  window_end?: string | null
}

export interface ChangeTransitionReq {
  action: ChangeAction
  result?: string
  reason?: string
  window_start?: string
  window_end?: string
  confirm_out_of_window?: boolean
}

export interface ApprovalReq {
  decision: ApprovalDecision
  comment: string
}

export const CHANGE_TRANSITIONS: ActionMap<ChangeStatus, ChangeAction> = {
  draft: [
    {
      action: 'submit_assessment',
      label: '提交风险评估',
      to: 'assessment',
      roles: ['requestor', 'admin']
    },
    { action: 'pre_authorize', label: '标准变更预授权', to: 'approved', roles: ['requestor', 'admin'] },
    {
      action: 'cancel',
      label: '撤销',
      to: 'cancelled',
      roles: ['requestor', 'change_manager', 'admin'],
      danger: true
    }
  ],
  assessment: [
    {
      action: 'submit_approval',
      label: '提交 CAB 审批',
      to: 'pending_approval',
      roles: ['requestor', 'change_manager']
    },
    {
      action: 'cancel',
      label: '撤销',
      to: 'cancelled',
      roles: ['requestor', 'change_manager', 'admin'],
      danger: true
    }
  ],
  pending_approval: [
    { action: 'approve', label: '审批通过', to: 'approved', roles: ['change_manager', 'admin'] },
    { action: 'reject', label: '驳回', to: 'rejected', roles: ['change_manager', 'admin'], danger: true }
  ],
  rejected: [{ action: 'revise', label: '修订重提', to: 'draft', roles: ['requestor'] }],
  approved: [
    { action: 'schedule', label: '排定窗口', to: 'scheduled', roles: ['change_manager', 'admin'] }
  ],
  scheduled: [
    {
      action: 'start_implement',
      label: '开始实施',
      to: 'implementing',
      roles: ['resolver', 'admin']
    },
    { action: 'cancel', label: '窗口前取消', to: 'cancelled', roles: ['change_manager', 'admin'], danger: true }
  ],
  implementing: [
    { action: 'complete', label: '实施成功', to: 'implemented', roles: ['resolver', 'admin'] },
    { action: 'rollback', label: '触发回滚', to: 'rolled_back', roles: ['resolver', 'admin'], danger: true }
  ],
  implemented: [
    { action: 'review', label: '进入回顾', to: 'review', roles: ['change_manager', 'admin'] }
  ],
  review: [
    { action: 'close', label: '通过关闭', to: 'closed', roles: ['change_manager', 'admin'] },
    { action: 'rollback', label: '判定回滚', to: 'rolled_back', roles: ['change_manager', 'admin'], danger: true }
  ],
  rolled_back: [{ action: 'resubmit', label: '重新申请', to: 'draft', roles: ['requestor'] }]
}

export const CHANGE_STATUS_LABELS: Record<ChangeStatus, string> = {
  draft: '草稿',
  assessment: '风险评估',
  pending_approval: '待审批',
  approved: '已批准',
  rejected: '已驳回',
  scheduled: '已排期',
  implementing: '实施中',
  implemented: '已实施',
  review: '回顾中',
  closed: '已关闭',
  rolled_back: '已回滚',
  cancelled: '已取消'
}

export const CHANGE_STATUS_OPTIONS = Object.keys(CHANGE_STATUS_LABELS).map((key) => ({
  label: CHANGE_STATUS_LABELS[key as ChangeStatus],
  value: key
}))

export const CHANGE_TYPE_LABELS: Record<ChangeType, string> = {
  standard: '标准变更',
  normal: '普通变更',
  emergency: '紧急变更'
}

export const CHANGE_TYPE_OPTIONS = [
  { label: '标准变更', value: 'standard' },
  { label: '普通变更', value: 'normal' },
  { label: '紧急变更', value: 'emergency' }
]

export const RISK_LABELS: Record<RiskLevel, string> = {
  high: '高',
  medium: '中',
  low: '低'
}

export const RISK_OPTIONS = [
  { label: '高', value: 'high' },
  { label: '中', value: 'medium' },
  { label: '低', value: 'low' }
]

export const APPROVAL_DECISION_LABELS: Record<ApprovalDecision, string> = {
  approved: '通过',
  rejected: '驳回'
}

export function changeActions(status: string, role: string | ''): ActionDef<ChangeAction>[] {
  const list = (CHANGE_TRANSITIONS as Record<string, ActionDef<ChangeAction>[] | undefined>)[status]
  if (!list || !role) return []
  return list.filter((item) => item.roles.includes(role as never))
}

/** 动作 -> 中文名（审计时间线标题本地化） */
export const CHANGE_ACTION_LABELS = actionLabelMap<ChangeAction>(CHANGE_TRANSITIONS)
