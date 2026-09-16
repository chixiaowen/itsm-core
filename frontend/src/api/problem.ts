/**
 * 问题 API（ARCHITECTURE §5.4）。
 */
import { del, get, post, put } from './http'
import type { PageResult } from '@/types/common'
import type {
  AggregateSuggestion,
  CreateProblemReq,
  KnownErrorReq,
  Problem,
  ProblemQuery,
  ProblemTransitionReq,
  UpdateProblemReq
} from '@/types/problem'

/** GET /problems */
export function fetchProblems(query: ProblemQuery): Promise<PageResult<Problem>> {
  return get<PageResult<Problem>>('/problems', query as unknown as Record<string, unknown>)
}

/** POST /problems 新建/聚合 */
export function createProblem(data: CreateProblemReq): Promise<Problem> {
  return post<Problem>('/problems', data)
}

/** GET /problems/:id */
export function fetchProblem(id: number): Promise<Problem> {
  return get<Problem>(`/problems/${id}`)
}

/** PUT /problems/:id */
export function updateProblem(id: number, data: UpdateProblemReq): Promise<Problem> {
  return put<Problem>(`/problems/${id}`, data)
}

/** DELETE /problems/:id */
export function deleteProblem(id: number): Promise<null> {
  return del<null>(`/problems/${id}`)
}

/** POST /problems/:id/transition */
export function transitionProblem(id: number, data: ProblemTransitionReq): Promise<Problem> {
  return post<Problem>(`/problems/${id}/transition`, data)
}

/** POST /problems/:id/known-error */
export function markKnownError(id: number, data: KnownErrorReq): Promise<Problem> {
  return post<Problem>(`/problems/${id}/known-error`, data)
}

/** POST /problems/:id/changes */
export function linkProblemChanges(id: number, changeIds: number[]): Promise<null> {
  return post<null>(`/problems/${id}/changes`, { change_ids: changeIds })
}

/** DELETE /problems/:id/changes/:changeId */
export function unlinkProblemChange(id: number, changeId: number): Promise<null> {
  return del<null>(`/problems/${id}/changes/${changeId}`)
}

/** GET /problems/aggregate-suggestions */
export function fetchAggregateSuggestions(ciId?: number, days = 30): Promise<AggregateSuggestion[]> {
  return get<AggregateSuggestion[]>('/problems/aggregate-suggestions', { ci_id: ciId, days })
}
