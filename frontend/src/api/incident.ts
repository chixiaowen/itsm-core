/**
 * 事件 API（ARCHITECTURE §5.3）。
 */
import { del, get, post, put } from './http'
import type { PageResult, Priority } from '@/types/common'
import type {
  CreateIncidentReq,
  EscalateReq,
  Incident,
  IncidentQuery,
  IncidentTransitionReq,
  PriorityMatrixCell,
  UpdateIncidentReq
} from '@/types/incident'
import type { Ticket } from '@/types/ticket'

/** GET /incidents */
export function fetchIncidents(query: IncidentQuery): Promise<PageResult<Incident>> {
  return get<PageResult<Incident>>('/incidents', query as unknown as Record<string, unknown>)
}

/** POST /incidents 上报 */
export function reportIncident(data: CreateIncidentReq): Promise<Incident> {
  return post<Incident>('/incidents', data)
}

/** GET /incidents/:id */
export function fetchIncident(id: number): Promise<Incident> {
  return get<Incident>(`/incidents/${id}`)
}

/** PUT /incidents/:id */
export function updateIncident(id: number, data: UpdateIncidentReq): Promise<Incident> {
  return put<Incident>(`/incidents/${id}`, data)
}

/** DELETE /incidents/:id */
export function deleteIncident(id: number): Promise<null> {
  return del<null>(`/incidents/${id}`)
}

/** POST /incidents/:id/transition */
export function transitionIncident(id: number, data: IncidentTransitionReq): Promise<Incident> {
  return post<Incident>(`/incidents/${id}/transition`, data)
}

/** POST /incidents/:id/priority 人工覆盖优先级 */
export function overrideIncidentPriority(id: number, priority: Priority): Promise<Incident> {
  return post<Incident>(`/incidents/${id}/priority`, { priority })
}

/** POST /incidents/:id/escalate */
export function escalateIncident(id: number, data: EscalateReq): Promise<Incident> {
  return post<Incident>(`/incidents/${id}/escalate`, data)
}

/** POST /incidents/:id/convert-to-ticket */
export function convertIncidentToTicket(id: number, title?: string): Promise<Ticket> {
  return post<Ticket>(`/incidents/${id}/convert-to-ticket`, { title })
}

/** POST /incidents/:id/link-ticket */
export function linkIncidentTicket(id: number, ticketId: number): Promise<Incident> {
  return post<Incident>(`/incidents/${id}/link-ticket`, { ticket_id: ticketId })
}

/** POST /incidents/:id/cis */
export function linkIncidentCis(id: number, ciIds: number[]): Promise<null> {
  return post<null>(`/incidents/${id}/cis`, { ci_ids: ciIds })
}

/** GET /incidents/priority-matrix 9 宫格矩阵 */
export function fetchPriorityMatrix(): Promise<PriorityMatrixCell[]> {
  return get<PriorityMatrixCell[]>('/incidents/priority-matrix')
}
