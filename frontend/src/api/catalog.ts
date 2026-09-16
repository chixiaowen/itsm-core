/**
 * 服务目录 API（ARCHITECTURE §5.6）。
 */
import { del, get, post, put } from './http'
import type { PageResult } from '@/types/common'
import type {
  OrderServiceReq,
  SaveServiceItemReq,
  ServiceCategory,
  ServiceCategoryNode,
  ServiceItem,
  ServiceItemQuery
} from '@/types/catalog'
import type { Ticket } from '@/types/ticket'

// ---------- 分类（管理台） ----------

/** GET /service-categories 分类树 */
export function fetchCategoryTree(): Promise<ServiceCategoryNode[]> {
  return get<ServiceCategoryNode[]>('/service-categories')
}

/** POST /service-categories */
export function createCategory(data: Partial<ServiceCategory>): Promise<ServiceCategory> {
  return post<ServiceCategory>('/service-categories', data)
}

/** PUT /service-categories/:id */
export function updateCategory(id: number, data: Partial<ServiceCategory>): Promise<ServiceCategory> {
  return put<ServiceCategory>(`/service-categories/${id}`, data)
}

/** DELETE /service-categories/:id */
export function deleteCategory(id: number): Promise<null> {
  return del<null>(`/service-categories/${id}`)
}

// ---------- 服务项（管理台） ----------

/** GET /service-items */
export function fetchServiceItems(query: ServiceItemQuery): Promise<PageResult<ServiceItem>> {
  return get<PageResult<ServiceItem>>('/service-items', query as unknown as Record<string, unknown>)
}

/** POST /service-items */
export function createServiceItem(data: SaveServiceItemReq): Promise<ServiceItem> {
  return post<ServiceItem>('/service-items', data)
}

/** GET /service-items/:id */
export function fetchServiceItem(id: number): Promise<ServiceItem> {
  return get<ServiceItem>(`/service-items/${id}`)
}

/** PUT /service-items/:id */
export function updateServiceItem(id: number, data: SaveServiceItemReq): Promise<ServiceItem> {
  return put<ServiceItem>(`/service-items/${id}`, data)
}

/** DELETE /service-items/:id 归档 */
export function archiveServiceItem(id: number): Promise<null> {
  return del<null>(`/service-items/${id}`)
}

/** POST /service-items/:id/publish */
export function publishServiceItem(id: number): Promise<ServiceItem> {
  return post<ServiceItem>(`/service-items/${id}/publish`)
}

/** POST /service-items/:id/offline */
export function offlineServiceItem(id: number): Promise<ServiceItem> {
  return post<ServiceItem>(`/service-items/${id}/offline`)
}

// ---------- 用户侧门户 ----------

/** GET /catalog/categories 用户侧分类（仅 published） */
export function fetchPortalCategories(): Promise<ServiceCategoryNode[]> {
  return get<ServiceCategoryNode[]>('/catalog/categories')
}

/** GET /catalog/items 用户侧服务项（仅 published） */
export function fetchPortalItems(params?: {
  category_id?: number
  keyword?: string
}): Promise<ServiceItem[]> {
  return get<ServiceItem[]>('/catalog/items', params)
}

/** POST /catalog/items/:id/order 下单 */
export function orderServiceItem(id: number, data: OrderServiceReq): Promise<Ticket> {
  return post<Ticket>(`/catalog/items/${id}/order`, data)
}
