/**
 * 资产域类型 + 生命周期状态机（PRD §5.7 / ARCHITECTURE §6.3.6）。
 */
import type { ActionDef, ActionMap, PageQuery } from './common'
import { actionLabelMap } from './common'

export type AssetStatus = 'planned' | 'in_stock' | 'in_use' | 'maintenance' | 'retired' | 'disposed'

export type AssetAction =
  | 'stock_in'
  | 'deploy'
  | 'retire'
  | 'maintain'
  | 'finish_maintain'
  | 'dispose'

export interface Asset {
  id: number
  asset_no: string
  name: string
  category: string
  status: AssetStatus
  ci_id: number | null
  user_id: number | null
  location: string
  vendor: string
  purchase_date: string | null
  warranty_end: string | null
  created_at: string
  updated_at: string
}

export interface AssetHistory {
  id: number
  asset_id: number
  from_status: string
  to_status: string
  remark: string
  actor_id: number
  created_at: string
}

export interface AssetQuery extends PageQuery {
  category?: string
  status?: AssetStatus | ''
  user_id?: number | ''
  warranty_before?: string
}

export interface SaveAssetReq {
  asset_no: string
  name: string
  category: string
  vendor?: string
  purchase_date?: string | null
  warranty_end?: string | null
}

export interface AssetTransitionReq {
  action: AssetAction
  remark?: string
  user_id?: number
  location?: string
  ci_id?: number
}

export const ASSET_TRANSITIONS: ActionMap<AssetStatus, AssetAction> = {
  planned: [
    { action: 'stock_in', label: '入库', to: 'in_stock', roles: ['cmdb_manager', 'admin'] }
  ],
  in_stock: [
    { action: 'deploy', label: '部署领用', to: 'in_use', roles: ['cmdb_manager', 'admin'] },
    { action: 'retire', label: '直接退役', to: 'retired', roles: ['cmdb_manager', 'admin'], danger: true }
  ],
  in_use: [
    { action: 'maintain', label: '送修', to: 'maintenance', roles: ['cmdb_manager', 'admin'] },
    { action: 'retire', label: '退役', to: 'retired', roles: ['cmdb_manager', 'admin'], danger: true }
  ],
  maintenance: [
    { action: 'finish_maintain', label: '维护完成', to: 'in_use', roles: ['cmdb_manager', 'admin'] }
  ],
  retired: [
    { action: 'dispose', label: '报废处置', to: 'disposed', roles: ['cmdb_manager', 'admin'], danger: true }
  ]
}

export const ASSET_STATUS_LABELS: Record<AssetStatus, string> = {
  planned: '规划',
  in_stock: '在库',
  in_use: '在用',
  maintenance: '维修中',
  retired: '已退役',
  disposed: '已报废'
}

export const ASSET_STATUS_OPTIONS = Object.keys(ASSET_STATUS_LABELS).map((key) => ({
  label: ASSET_STATUS_LABELS[key as AssetStatus],
  value: key
}))

export const ASSET_CATEGORY_OPTIONS = [
  { label: '服务器', value: 'server' },
  { label: '网络设备', value: 'network' },
  { label: '终端设备', value: 'terminal' },
  { label: '软件资产', value: 'software' },
  { label: '其他', value: 'other' }
]

export function assetActions(status: string, role: string | ''): ActionDef<AssetAction>[] {
  const list = (ASSET_TRANSITIONS as Record<string, ActionDef<AssetAction>[] | undefined>)[status]
  if (!list || !role) return []
  return list.filter((item) => item.roles.includes(role as never))
}

/** 动作 -> 中文名（审计时间线标题本地化） */
export const ASSET_ACTION_LABELS = actionLabelMap<AssetAction>(ASSET_TRANSITIONS)
