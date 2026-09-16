/**
 * 服务目录域类型 + 服务项状态机 + 动态表单 Schema（PRD §5.6、§6.1、§7.7）。
 */
import type { ActionDef, ActionMap, PageQuery, Priority } from './common'

export type ServiceItemStatus = 'draft' | 'pending_approval' | 'published' | 'offline' | 'archived'

export type ServiceItemAction =
  | 'submit_review'
  | 'archive'
  | 'publish'
  | 'reject'
  | 'edit'
  | 'offline'
  | 'republish'

export interface ServiceCategory {
  id: number
  name: string
  parent_id: number | null
  sort_order: number
  created_at: string
  updated_at: string
}

export interface ServiceCategoryNode extends ServiceCategory {
  children: ServiceCategoryNode[]
}

/** 动态表单字段类型（DynamicForm 支持渲染的类型） */
export type FormFieldType = 'text' | 'number' | 'select' | 'date' | 'textarea'

export interface FormField {
  name: string
  label: string
  type: FormFieldType
  required: boolean
  options?: string[]
  placeholder?: string
  sort?: number
}

export interface ServiceItem {
  id: number
  name: string
  description: string
  status: ServiceItemStatus
  category_id: number
  sla_policy_id: number | null
  default_priority: Priority
  requires_approval: boolean
  form_schema: string
  created_at: string
  updated_at: string
}

export interface ServiceItemQuery extends PageQuery {
  status?: ServiceItemStatus | ''
  category_id?: number | ''
}

export interface SaveServiceItemReq {
  name: string
  description?: string
  category_id: number
  sla_policy_id?: number | null
  default_priority?: Priority
  requires_approval?: boolean
  form_schema?: string
}

export interface OrderServiceReq {
  form_data: Record<string, string | number | null>
  title?: string
}

export const SERVICE_ITEM_TRANSITIONS: ActionMap<ServiceItemStatus, ServiceItemAction> = {
  draft: [
    { action: 'submit_review', label: '提交审核', to: 'pending_approval', roles: ['admin'] },
    { action: 'archive', label: '归档', to: 'archived', roles: ['admin'], danger: true }
  ],
  pending_approval: [
    { action: 'publish', label: '发布', to: 'published', roles: ['admin'] },
    { action: 'reject', label: '驳回', to: 'draft', roles: ['admin'], danger: true }
  ],
  published: [
    { action: 'edit', label: '编辑（退回草稿）', to: 'draft', roles: ['admin'] },
    { action: 'offline', label: '下线', to: 'offline', roles: ['admin'] }
  ],
  offline: [
    { action: 'republish', label: '重新上架', to: 'published', roles: ['admin'] },
    { action: 'archive', label: '归档', to: 'archived', roles: ['admin'], danger: true }
  ]
}

export const SERVICE_ITEM_STATUS_LABELS: Record<ServiceItemStatus, string> = {
  draft: '草稿',
  pending_approval: '待审核',
  published: '已发布',
  offline: '已下线',
  archived: '已归档'
}

export const SERVICE_ITEM_STATUS_OPTIONS = Object.keys(SERVICE_ITEM_STATUS_LABELS).map((key) => ({
  label: SERVICE_ITEM_STATUS_LABELS[key as ServiceItemStatus],
  value: key
}))

export const FORM_FIELD_TYPE_OPTIONS = [
  { label: '单行文本', value: 'text' },
  { label: '数字', value: 'number' },
  { label: '下拉选择', value: 'select' },
  { label: '日期', value: 'date' },
  { label: '多行文本', value: 'textarea' }
]

/** 解析 form_schema（JSON 字符串）为字段数组；容错非法 JSON */
export function parseFormSchema(schema?: string | null): FormField[] {
  if (!schema) return []
  try {
    const parsed = JSON.parse(schema)
    if (!Array.isArray(parsed)) return []
    return parsed as FormField[]
  } catch {
    return []
  }
}

export function serviceItemActions(status: string, role: string | ''): ActionDef<ServiceItemAction>[] {
  const list = (SERVICE_ITEM_TRANSITIONS as Record<string, ActionDef<ServiceItemAction>[] | undefined>)[
    status
  ]
  if (!list || !role) return []
  return list.filter((item) => item.roles.includes(role as never))
}
