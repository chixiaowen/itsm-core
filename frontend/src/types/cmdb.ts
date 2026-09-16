/**
 * CMDB 域类型：CI / CI 关系 / 拓扑（PRD §5.7 / ARCHITECTURE §4.8、§5.7）。
 */
import type { PageQuery } from './common'

export type CiStatus = 'planned' | 'in_stock' | 'in_use' | 'maintenance' | 'retired' | 'disposed'

export type CiType = 'server' | 'network' | 'database' | 'application' | 'terminal' | 'other'

export type RelationType = 'depends_on' | 'contains' | 'connects_to'

export interface CI {
  id: number
  code: string
  name: string
  ci_type: CiType
  status: CiStatus
  attrs: string
  owner_id: number | null
  created_at: string
  updated_at: string
  relation_count?: number
}

export interface CIRelation {
  id: number
  source_ci_id: number
  target_ci_id: number
  relation_type: RelationType
  created_at: string
  target_name?: string
  target_code?: string
}

export interface CiQuery extends PageQuery {
  ci_type?: CiType | ''
  status?: CiStatus | ''
  owner_id?: number | ''
  keyword?: string
}

export interface SaveCiReq {
  code: string
  name: string
  ci_type: CiType
  status?: CiStatus
  owner_id?: number | null
  attrs?: string
}

export interface CreateRelationReq {
  target_ci_id: number
  relation_type: RelationType
}

/** 拓扑节点 */
export interface TopologyNode {
  id: number
  code: string
  name: string
  ci_type: CiType
  status: CiStatus
  level?: number
}

/** 拓扑边 */
export interface TopologyEdge {
  source: number
  target: number
  relation_type: RelationType
}

/** 拓扑响应 {nodes, edges} */
export interface Topology {
  nodes: TopologyNode[]
  edges: TopologyEdge[]
}

export const CI_STATUS_LABELS: Record<CiStatus, string> = {
  planned: '规划',
  in_stock: '在库',
  in_use: '在用',
  maintenance: '维修中',
  retired: '已退役',
  disposed: '已报废'
}

export const CI_STATUS_OPTIONS = Object.keys(CI_STATUS_LABELS).map((key) => ({
  label: CI_STATUS_LABELS[key as CiStatus],
  value: key
}))

export const CI_TYPE_LABELS: Record<CiType, string> = {
  server: '服务器',
  network: '网络设备',
  database: '数据库',
  application: '应用',
  terminal: '终端',
  other: '其他'
}

export const CI_TYPE_OPTIONS = Object.keys(CI_TYPE_LABELS).map((key) => ({
  label: CI_TYPE_LABELS[key as CiType],
  value: key
}))

export const RELATION_TYPE_LABELS: Record<RelationType, string> = {
  depends_on: '依赖',
  contains: '包含',
  connects_to: '连接'
}

export const RELATION_TYPE_OPTIONS = [
  { label: '依赖', value: 'depends_on' },
  { label: '包含', value: 'contains' },
  { label: '连接', value: 'connects_to' }
]
