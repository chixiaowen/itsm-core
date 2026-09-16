/**
 * 平台域 API：用户 / 角色 / SLA 策略 / 审计 / 评论 / 附件（ARCHITECTURE §5.1）。
 */
import { del, download, get, post, put, upload } from './http'
import type { PageResult } from '@/types/common'
import type {
  Attachment,
  AuditLog,
  AuditLogQuery,
  Comment,
  CreateCommentReq,
  CreateUserReq,
  EntityAuditQuery,
  RoleInfo,
  SaveSLAPolicyReq,
  SLAPolicy,
  UpdateUserReq,
  User,
  UserOption
} from '@/types/platform'

/** GET /users/options 用户下拉项（任意已登录用户可访问；role 为空表示不过滤） */
export function listUserOptions(params?: { role?: string }): Promise<UserOption[]> {
  return get<UserOption[]>('/users/options', params as Record<string, unknown> | undefined)
}

/** GET /users */
export function fetchUsers(params?: Record<string, unknown>): Promise<PageResult<User>> {
  return get<PageResult<User>>('/users', params)
}

/** POST /users */
export function createUser(data: CreateUserReq): Promise<User> {
  return post<User>('/users', data)
}

/** GET /users/:id */
export function fetchUser(id: number): Promise<User> {
  return get<User>(`/users/${id}`)
}

/** PUT /users/:id */
export function updateUser(id: number, data: UpdateUserReq): Promise<User> {
  return put<User>(`/users/${id}`, data)
}

/** DELETE /users/:id */
export function deleteUser(id: number): Promise<null> {
  return del<null>(`/users/${id}`)
}

/** GET /roles 角色与权限点矩阵 */
export function fetchRoles(): Promise<RoleInfo[]> {
  return get<RoleInfo[]>('/roles')
}

/** GET /sla-policies */
export function fetchSlaPolicies(): Promise<SLAPolicy[]> {
  return get<SLAPolicy[]>('/sla-policies')
}

/** POST /sla-policies */
export function createSlaPolicy(data: SaveSLAPolicyReq): Promise<SLAPolicy> {
  return post<SLAPolicy>('/sla-policies', data)
}

/** PUT /sla-policies/:id */
export function updateSlaPolicy(id: number, data: SaveSLAPolicyReq): Promise<SLAPolicy> {
  return put<SLAPolicy>(`/sla-policies/${id}`, data)
}

/** DELETE /sla-policies/:id */
export function deleteSlaPolicy(id: number): Promise<null> {
  return del<null>(`/sla-policies/${id}`)
}

/** GET /audit-logs */
export function fetchAuditLogs(query: AuditLogQuery): Promise<PageResult<AuditLog>> {
  return get<PageResult<AuditLog>>('/audit-logs', query as unknown as Record<string, unknown>)
}

/**
 * GET /audit-logs?entity_type=&entity_id= 实体维度审计（详情页状态流转时间线）。
 * 使用静默模式：403（无权限）等失败不弹错误，由调用方隐藏「状态流转」区块。
 */
export function fetchEntityAuditLogs(query: EntityAuditQuery): Promise<PageResult<AuditLog>> {
  return get<PageResult<AuditLog>>('/audit-logs', query as unknown as Record<string, unknown>, {
    silent: true
  })
}

/** GET /comments */
export function fetchComments(params: {
  biz_type: string
  biz_id: number
  include_internal?: boolean
}): Promise<Comment[]> {
  return get<Comment[]>('/comments', params)
}

/** POST /comments */
export function createComment(data: CreateCommentReq): Promise<Comment> {
  return post<Comment>('/comments', data)
}

/** POST /attachments（multipart） */
export function uploadAttachment(formData: FormData): Promise<Attachment> {
  return upload<Attachment>('/attachments', formData)
}

/** GET /attachments/:id/download */
export function downloadAttachment(id: number): Promise<Blob> {
  return download(`/attachments/${id}/download`)
}

/** DELETE /attachments/:id */
export function deleteAttachment(id: number): Promise<null> {
  return del<null>(`/attachments/${id}`)
}
