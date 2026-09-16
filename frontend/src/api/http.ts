/**
 * axios 实例与统一封装（ARCHITECTURE §14.4）：
 * - baseURL 取 VITE_API_BASE，默认 /api/v1
 * - 请求拦截：注入 Authorization: Bearer <jwt>
 * - 响应拦截：解包 {code,message,data}；code!==0 弹 ElMessage 并 reject
 * - 401：清除 token 并跳转登录页
 */
import axios, { type AxiosRequestConfig, type AxiosInstance } from 'axios'
import { ElMessage } from 'element-plus'
import type { ApiResponse } from '@/types/common'

const TOKEN_KEY = 'itsm_token'

/** 请求级扩展配置 */
export interface HttpConfig extends AxiosRequestConfig {
  /**
   * 静默模式：失败时不弹 ElMessage。
   * 用于后台旁路请求（如详情页审计时间线），由调用方自行优雅降级。
   */
  silent?: boolean
}

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? ''
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

/** 未登录跳转（避免与 router 形成静态循环依赖，使用动态 import） */
function redirectToLogin(): void {
  clearToken()
  if (window.location.hash !== '#/login' && window.location.pathname !== '/login') {
    import('@/router')
      .then((mod) => {
        void mod.default.push({ path: '/login' })
      })
      .catch(() => {
        window.location.href = '/login'
      })
  }
}

const http: AxiosInstance = axios.create({
  baseURL: import.meta.env.VITE_API_BASE ?? '/api/v1',
  timeout: 20000
})

http.interceptors.request.use((config) => {
  const token = getToken()
  if (token) {
    config.headers = config.headers ?? {}
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

http.interceptors.response.use(
  (response) => {
    const body = response.data as ApiResponse<unknown> | undefined
    // 非统一响应体（如文件流）直接返回
    if (!body || typeof body !== 'object' || !('code' in body)) {
      return response.data
    }
    if (body.code === 0) {
      return body.data
    }
    const silent = (response.config as HttpConfig | undefined)?.silent
    if (!silent) ElMessage.error(body.message || '请求失败')
    return Promise.reject(new Error(body.message || '请求失败'))
  },
  (error) => {
    const status: number | undefined = error?.response?.status
    const body = error?.response?.data as ApiResponse<unknown> | undefined
    const message = body?.message || error?.message || '网络异常，请稍后重试'
    const silent = (error?.config as HttpConfig | undefined)?.silent
    if (status === 401) {
      ElMessage.error(message || '登录已过期，请重新登录')
      redirectToLogin()
    } else if (!silent) {
      ElMessage.error(message)
    }
    return Promise.reject(error)
  }
)

/** GET */
export function get<T>(url: string, params?: Record<string, unknown>, config?: HttpConfig): Promise<T> {
  return http.get(url, { params, ...config }) as unknown as Promise<T>
}

/** POST */
export function post<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
  return http.post(url, data, config) as unknown as Promise<T>
}

/** PUT */
export function put<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
  return http.put(url, data, config) as unknown as Promise<T>
}

/** DELETE */
export function del<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
  return http.delete(url, config) as unknown as Promise<T>
}

/** 上传 multipart 表单 */
export function upload<T>(url: string, formData: FormData): Promise<T> {
  return http.post(url, formData, {
    headers: { 'Content-Type': 'multipart/form-data' }
  }) as unknown as Promise<T>
}

/** 下载获取 Blob（响应拦截对非统一响应体直接透传） */
export function download(url: string, params?: Record<string, unknown>): Promise<Blob> {
  return http.get(url, { params, responseType: 'blob' }) as unknown as Promise<Blob>
}

export default http
