/**
 * 问题域类型 + 状态机流转映射（PRD §5.4 / ARCHITECTURE §6.3.3）。
 */
import type { ActionDef, ActionMap, PageQuery } from './common'
import { actionLabelMap } from './common'

export type ProblemStatus =
  | 'new'
  | 'triage'
  | 'investigating'
  | 'known_error'
  | 'resolved'
  | 'closed'
  | 'cancelled'

export type ProblemAction =
  | 'triage'
  | 'cancel'
  | 'investigate'
  | 'mark_known_error'
  | 'update_workaround'
  | 'resolve'
  | 'close'
  | 'recur'

export type ProblemSource = 'aggregate' | 'manual'

export interface Problem {
  id: number
  code: string
  title: string
  description: string
  status: ProblemStatus
  source: ProblemSource
  root_cause: string
  analysis: string
  symptom: string
  workaround: string
  no_change_reason: string
  assignee_id: number | null
  creator_id: number
  resolved_at: string | null
  closed_at: string | null
  created_at: string
  updated_at: string
  incident_ids?: number[]
  change_ids?: number[]
}

export interface ProblemQuery extends PageQuery {
  status?: ProblemStatus | ''
  assignee_id?: number | ''
  known_error?: boolean | ''
}

export interface CreateProblemReq {
  title: string
  description?: string
  source: ProblemSource
  incident_ids?: number[]
}

export interface UpdateProblemReq {
  title?: string
  description?: string
  symptom?: string
  analysis?: string
  root_cause?: string
}

export interface ProblemTransitionReq {
  action: ProblemAction
  workaround?: string
  no_change_reason?: string
  reason?: string
}

export interface KnownErrorReq {
  root_cause: string
  workaround: string
}

export interface AggregateSuggestion {
  ci_id: number
  ci_name: string
  incident_count: number
  incident_ids: number[]
}

export const PROBLEM_TRANSITIONS: ActionMap<ProblemStatus, ProblemAction> = {
  new: [
    { action: 'triage', label: '分诊', to: 'triage', roles: ['problem_manager', 'admin'] },
    { action: 'cancel', label: '取消', to: 'cancelled', roles: ['problem_manager', 'admin'], danger: true }
  ],
  triage: [
    { action: 'investigate', label: '开始调查', to: 'investigating', roles: ['problem_manager', 'admin'] },
    { action: 'cancel', label: '取消', to: 'cancelled', roles: ['problem_manager', 'admin'], danger: true }
  ],
  investigating: [
    {
      action: 'mark_known_error',
      label: '标记已知错误',
      to: 'known_error',
      roles: ['problem_manager', 'admin']
    },
    { action: 'resolve', label: '解决', to: 'resolved', roles: ['problem_manager', 'admin'] }
  ],
  known_error: [
    {
      action: 'update_workaround',
      label: '更新规避方案',
      to: 'known_error',
      roles: ['problem_manager', 'admin']
    },
    { action: 'resolve', label: '解决', to: 'resolved', roles: ['problem_manager', 'admin'] }
  ],
  resolved: [
    { action: 'close', label: '关闭', to: 'closed', roles: ['problem_manager', 'admin'] },
    { action: 'recur', label: '复发回退', to: 'investigating', roles: ['problem_manager', 'admin'] }
  ]
}

export const PROBLEM_STATUS_LABELS: Record<ProblemStatus, string> = {
  new: '新建',
  triage: '待分诊',
  investigating: '调查中',
  known_error: '已知错误',
  resolved: '已解决',
  closed: '已关闭',
  cancelled: '已取消'
}

export const PROBLEM_STATUS_OPTIONS = Object.keys(PROBLEM_STATUS_LABELS).map((key) => ({
  label: PROBLEM_STATUS_LABELS[key as ProblemStatus],
  value: key
}))

export const PROBLEM_SOURCE_LABELS: Record<ProblemSource, string> = {
  aggregate: '事件聚合',
  manual: '手动创建'
}

export function problemActions(status: string, role: string | ''): ActionDef<ProblemAction>[] {
  const list = (PROBLEM_TRANSITIONS as Record<string, ActionDef<ProblemAction>[] | undefined>)[status]
  if (!list || !role) return []
  return list.filter((item) => item.roles.includes(role as never))
}

/** 动作 -> 中文名（审计时间线标题本地化） */
export const PROBLEM_ACTION_LABELS = actionLabelMap<ProblemAction>(PROBLEM_TRANSITIONS)
