/**
 * 平台域类型：用户 / 角色 / SLA 策略 / 审计 / 评论 / 附件。
 * 与 ARCHITECTURE §4.2、§5.1 对齐。
 */
import type { Priority, Role } from './common'

export type UserStatus = 'active' | 'disabled'

export interface User {
  id: number
  username: string
  display_name: string
  role: Role
  email: string
  status: UserStatus
  created_at: string
  updated_at: string
}

/**
 * 候选用户下拉项（GET /users/options）。
 * 后端仅返回 id / display_name / role 三个非敏感字段，任意已登录用户可访问。
 */
export interface UserOption {
  id: number
  display_name: string
  role: Role
}

export interface LoginReq {
  username: string
  password: string
}

export interface LoginResp {
  token: string
  expires_at: string
  user: User
}

export interface RoleInfo {
  role: Role
  name: string
  permissions: string[]
}

export interface SLAPolicy {
  id: number
  name: string
  priority: Priority
  response_minutes: number
  resolve_minutes: number
  pause_on_pending: boolean
  created_at: string
  updated_at: string
}

export interface AuditLog {
  id: number
  actor_id: number
  action: string
  biz_type: string
  biz_id: number
  from_status: string
  to_status: string
  before_value: string
  after_value: string
  client_ip: string
  created_at: string
}

/** 评论多态业务类型 */
export type BizType = 'ticket' | 'incident' | 'problem' | 'change'

export interface Comment {
  id: number
  biz_type: BizType
  biz_id: number
  author_id: number
  content: string
  is_internal: boolean
  created_at: string
  updated_at: string
}

export interface Attachment {
  id: number
  biz_type: BizType
  biz_id: number
  filename: string
  file_path: string
  size: number
  mime_type: string
  uploader_id: number
  created_at: string
}

export interface CreateUserReq {
  username: string
  display_name: string
  role: Role
  password: string
  email?: string
}

export interface UpdateUserReq {
  display_name?: string
  role?: Role
  password?: string
  status?: UserStatus
}

export interface SaveSLAPolicyReq {
  name: string
  priority: Priority
  response_minutes: number
  resolve_minutes: number
  pause_on_pending: boolean
}

export interface CreateCommentReq {
  biz_type: BizType
  biz_id: number
  content: string
  is_internal: boolean
}

export interface AuditLogQuery {
  actor_id?: number
  biz_type?: string
  biz_id?: number
  entity_type?: string
  entity_id?: number
  action?: string
  from?: string
  to?: string
  page?: number
  page_size?: number
}

/**
 * 实体维度审计查询（详情页「状态流转时间线」用）。
 * entity_type/entity_id 与 biz_type/biz_id 在服务端互为别名。
 */
export interface EntityAuditQuery {
  entity_type: string
  entity_id: number
  page?: number
  page_size?: number
}
