/**
 * CMDB API：CI / 关系 / 拓扑 / CI 类型（ARCHITECTURE §5.7）。
 */
import { del, get, post, put } from './http'
import type { PageResult } from '@/types/common'
import type {
  CI,
  CIRelation,
  CiQuery,
  CreateRelationReq,
  SaveCiReq,
  Topology
} from '@/types/cmdb'

/** GET /cis */
export function fetchCis(query: CiQuery): Promise<PageResult<CI>> {
  return get<PageResult<CI>>('/cis', query as unknown as Record<string, unknown>)
}

/** POST /cis */
export function createCi(data: SaveCiReq): Promise<CI> {
  return post<CI>('/cis', data)
}

/** GET /cis/:id */
export function fetchCi(id: number): Promise<CI> {
  return get<CI>(`/cis/${id}`)
}

/** PUT /cis/:id */
export function updateCi(id: number, data: SaveCiReq): Promise<CI> {
  return put<CI>(`/cis/${id}`, data)
}

/** DELETE /cis/:id */
export function deleteCi(id: number): Promise<null> {
  return del<null>(`/cis/${id}`)
}

/** GET /cis/:id/topology 拓扑展开 */
export function fetchTopology(id: number, depth = 2, direction = 'both'): Promise<Topology> {
  return get<Topology>(`/cis/${id}/topology`, { depth, direction })
}

/** POST /cis/:id/relations 新增关系 */
export function createRelation(id: number, data: CreateRelationReq): Promise<CIRelation> {
  return post<CIRelation>(`/cis/${id}/relations`, data)
}

/** DELETE /cis/:id/relations/:relId */
export function deleteRelation(id: number, relId: number): Promise<null> {
  return del<null>(`/cis/${id}/relations/${relId}`)
}

/** GET /ci-types CI 类型枚举 */
export function fetchCiTypes(): Promise<string[]> {
  return get<string[]>('/ci-types')
}
