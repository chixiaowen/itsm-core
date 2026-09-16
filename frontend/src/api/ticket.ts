/**
 * 工单 API（ARCHITECTURE §5.2）。
 */
import { del, get, post, put } from './http'
import type { PageResult } from '@/types/common'
import type {
  CreateTicketReq,
  RatingReq,
  Ticket,
  TicketCategory,
  TicketCategoryNode,
  TicketQuery,
  TransitionReq,
  UpdateTicketReq
} from '@/types/ticket'

/** GET /tickets */
export function fetchTickets(query: TicketQuery): Promise<PageResult<Ticket>> {
  return get<PageResult<Ticket>>('/tickets', query as unknown as Record<string, unknown>)
}

/** POST /tickets */
export function createTicket(data: CreateTicketReq): Promise<Ticket> {
  return post<Ticket>('/tickets', data)
}

/** GET /tickets/:id */
export function fetchTicket(id: number): Promise<Ticket> {
  return get<Ticket>(`/tickets/${id}`)
}

/** PUT /tickets/:id */
export function updateTicket(id: number, data: UpdateTicketReq): Promise<Ticket> {
  return put<Ticket>(`/tickets/${id}`, data)
}

/** DELETE /tickets/:id */
export function deleteTicket(id: number): Promise<null> {
  return del<null>(`/tickets/${id}`)
}

/** POST /tickets/:id/transition 统一状态流转 */
export function transitionTicket(id: number, data: TransitionReq): Promise<Ticket> {
  return post<Ticket>(`/tickets/${id}/transition`, data)
}

/** POST /tickets/:id/assign 快捷指派 */
export function assignTicket(id: number, assigneeId: number): Promise<Ticket> {
  return post<Ticket>(`/tickets/${id}/assign`, { assignee_id: assigneeId })
}

/** POST /tickets/:id/rating 满意度评价（仅一次） */
export function rateTicket(id: number, data: RatingReq): Promise<Ticket> {
  return post<Ticket>(`/tickets/${id}/rating`, data)
}

/** POST /tickets/:id/comment 公开回复/内部备注 */
export function commentTicket(id: number, content: string, isInternal: boolean): Promise<unknown> {
  return post<unknown>(`/tickets/${id}/comment`, { content, is_internal: isInternal })
}

/** POST /tickets/:id/cis 关联 CI */
export function linkTicketCis(id: number, ciIds: number[]): Promise<null> {
  return post<null>(`/tickets/${id}/cis`, { ci_ids: ciIds })
}

/** DELETE /tickets/:id/cis/:ciId 解除 CI */
export function unlinkTicketCi(id: number, ciId: number): Promise<null> {
  return del<null>(`/tickets/${id}/cis/${ciId}`)
}

/** GET /ticket-categories 分类树 */
export function fetchTicketCategories(): Promise<TicketCategoryNode[]> {
  return get<TicketCategoryNode[]>('/ticket-categories')
}

/** POST /ticket-categories */
export function createTicketCategory(data: Partial<TicketCategory>): Promise<TicketCategory> {
  return post<TicketCategory>('/ticket-categories', data)
}

/** PUT /ticket-categories/:id */
export function updateTicketCategory(id: number, data: Partial<TicketCategory>): Promise<TicketCategory> {
  return put<TicketCategory>(`/ticket-categories/${id}`, data)
}

/** DELETE /ticket-categories/:id */
export function deleteTicketCategory(id: number): Promise<null> {
  return del<null>(`/ticket-categories/${id}`)
}
