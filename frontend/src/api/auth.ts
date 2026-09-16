/**
 * 认证 API（ARCHITECTURE §5.1）。
 */
import { get, post } from './http'
import type { LoginReq, LoginResp, User } from '@/types/platform'

/** POST /auth/login 登录签发 JWT */
export function login(data: LoginReq): Promise<LoginResp> {
  return post<LoginResp>('/auth/login', data)
}

/** GET /auth/me 当前用户 */
export function fetchMe(): Promise<User> {
  return get<User>('/auth/me')
}

/** POST /auth/logout 登出（前端清 token） */
export function logout(): Promise<null> {
  return post<null>('/auth/logout')
}
