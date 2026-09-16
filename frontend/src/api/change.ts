/**
 * 变更 API（ARCHITECTURE §5.5）。
 */
import { del, get, post, put } from './http'
import type { PageResult } from '@/types/common'
import type {
  ApprovalReq,
  Change,
  ChangeApproval,
  ChangeQuery,
  ChangeTransitionReq,
  CreateChangeReq,
  UpdateChangeReq
} from '@/types/change'

/** GET /changes */
export function fetchChanges(query: ChangeQuery): Promise<PageResult<Change>> {
  return get<PageResult<Change>>('/changes', query as unknown as Record<string, unknown>)
}

/** POST /changes */
export function createChange(data: CreateChangeReq): Promise<Change> {
  return post<Change>('/changes', data)
}

/** GET /changes/:id */
export function fetchChange(id: number): Promise<Change> {
  return get<Change>(`/changes/${id}`)
}

/** PUT /changes/:id */
export function updateChange(id: number, data: UpdateChangeReq): Promise<Change> {
  return put<Change>(`/changes/${id}`, data)
}

/** DELETE /changes/:id */
export function deleteChange(id: number): Promise<null> {
  return del<null>(`/changes/${id}`)
}

/** POST /changes/:id/transition */
export function transitionChange(id: number, data: ChangeTransitionReq): Promise<Change> {
  return post<Change>(`/changes/${id}/transition`, data)
}

/** POST /changes/:id/approvals CAB 审批 */
export function approveChange(id: number, data: ApprovalReq): Promise<Change> {
  return post<Change>(`/changes/${id}/approvals`, data)
}

/** GET /changes/:id/approvals 审批记录 */
export function fetchChangeApprovals(id: number): Promise<ChangeApproval[]> {
  return get<ChangeApproval[]>(`/changes/${id}/approvals`)
}
