/**
 * 用户 Store：token / 用户信息 / 权限点（ARCHITECTURE §14.3）。
 *
 * permissions 优先取后端 /roles 矩阵（admin），其余角色用本地矩阵兜底，
 * 供 Dashboard 等展示当前用户权限点。
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as authApi from '@/api/auth'
import * as platformApi from '@/api/platform'
import { clearToken, getToken, setToken } from '@/api/http'
import type { LoginReq, User } from '@/types/platform'
import type { Role } from '@/types/common'

/**
 * 权限点常量。
 *
 * ⚠️ 真值来源为后端 `internal/pkg/role/role.go` 的 `AllPermissions`（权限点全集）。
 * 后端新增/删除权限点时，此处必须同步，否则会产生「指向已不存在权限点」的悬空引用。
 * 命名遵循 `perm.<domain>.<action>`；状态机类动作不单列权限点（统一走通用 transition 端点）。
 */
export const PERMS = {
  // 工单
  ticketCreate: 'perm.ticket.create',
  ticketViewAll: 'perm.ticket.view_all',
  ticketAssign: 'perm.ticket.assign',
  ticketHandle: 'perm.ticket.handle',
  ticketRate: 'perm.ticket.rate',
  // 事件
  incidentReport: 'perm.incident.report',
  incidentEscalate: 'perm.incident.escalate',
  incidentConvert: 'perm.incident.convert',
  // 问题
  problemCreate: 'perm.problem.create',
  problemRca: 'perm.problem.rca',
  // 变更
  changeSubmit: 'perm.change.submit',
  changeApprove: 'perm.change.approve',
  // 服务目录
  catalogManage: 'perm.catalog.manage',
  catalogOrder: 'perm.catalog.order',
  // 配置管理 / 资产
  cmdbManage: 'perm.cmdb.manage',
  assetManage: 'perm.asset.manage',
  // 平台
  slaManage: 'perm.sla.manage',
  userManage: 'perm.user.manage',
  auditView: 'perm.audit.view'
} as const

/** 权限点全集（对齐后端 AllPermissions） */
const ALL_PERMISSIONS: string[] = Object.values(PERMS)

/** admin 实际持有集（对齐后端 AdminPermissions：刻意不含 perm.ticket.rate） */
const ADMIN_PERMISSIONS: string[] = ALL_PERMISSIONS.filter((perm) => perm !== PERMS.ticketRate)

/**
 * 角色 → 权限点矩阵（本地兜底，逐条对齐后端 `role.go` 的 `rolePermissions`）。
 * admin 兜底 = ADMIN_PERMISSIONS（与后端 AdminPermissions 一致）。
 */
const ROLE_PERMISSIONS: Record<Role, string[]> = {
  requestor: [PERMS.ticketCreate, PERMS.ticketRate, PERMS.incidentReport, PERMS.catalogOrder],
  agent: [
    PERMS.ticketCreate,
    PERMS.ticketViewAll,
    PERMS.ticketAssign,
    PERMS.ticketHandle,
    PERMS.incidentReport,
    PERMS.incidentEscalate,
    PERMS.incidentConvert,
    PERMS.problemCreate,
    PERMS.changeSubmit,
    PERMS.catalogOrder
  ],
  resolver: [
    PERMS.ticketCreate,
    PERMS.ticketViewAll,
    PERMS.ticketAssign,
    PERMS.ticketHandle,
    PERMS.incidentReport,
    PERMS.incidentEscalate,
    PERMS.incidentConvert,
    PERMS.problemCreate,
    PERMS.problemRca,
    PERMS.changeSubmit,
    PERMS.catalogOrder,
    PERMS.cmdbManage
  ],
  problem_manager: [
    PERMS.ticketCreate,
    PERMS.ticketViewAll,
    PERMS.incidentReport,
    PERMS.incidentEscalate,
    PERMS.problemCreate,
    PERMS.problemRca,
    PERMS.changeSubmit,
    PERMS.changeApprove,
    PERMS.catalogOrder,
    PERMS.cmdbManage
  ],
  change_manager: [
    PERMS.ticketCreate,
    PERMS.ticketViewAll,
    PERMS.incidentReport,
    PERMS.changeSubmit,
    PERMS.changeApprove,
    PERMS.catalogOrder
  ],
  cmdb_manager: [
    PERMS.ticketCreate,
    PERMS.ticketViewAll,
    PERMS.incidentReport,
    PERMS.changeSubmit,
    PERMS.catalogOrder,
    PERMS.cmdbManage,
    PERMS.assetManage
  ],
  admin: ADMIN_PERMISSIONS
}

export const useUserStore = defineStore('user', () => {
  const token = ref<string>(getToken())
  const profile = ref<User | null>(null)
  const permissions = ref<string[]>([])

  const role = computed<Role | ''>(() => profile.value?.role ?? '')
  const isLogin = computed<boolean>(() => Boolean(token.value))

  async function login(payload: LoginReq): Promise<User> {
    const resp = await authApi.login(payload)
    token.value = resp.token
    setToken(resp.token)
    profile.value = resp.user
    await loadPermissions()
    return resp.user
  }

  async function fetchMe(): Promise<User> {
    const user = await authApi.fetchMe()
    profile.value = user
    return user
  }

  /** 加载权限点：admin 尝试从 /roles 获取；失败或非 admin 使用本地矩阵 */
  async function loadPermissions(): Promise<void> {
    const currentRole = role.value
    if (!currentRole) {
      permissions.value = []
      return
    }
    if (currentRole === 'admin') {
      try {
        const roles = await platformApi.fetchRoles()
        const mine = roles.find((item) => item.role === 'admin')
        permissions.value = mine?.permissions?.length ? mine.permissions : ADMIN_PERMISSIONS
        return
      } catch {
        permissions.value = ADMIN_PERMISSIONS
        return
      }
    }
    permissions.value = ROLE_PERMISSIONS[currentRole] ?? []
  }

  function hasRole(roles: Role[]): boolean {
    return Boolean(role.value) && roles.includes(role.value as Role)
  }

  function logout(): void {
    clearToken()
    token.value = ''
    profile.value = null
    permissions.value = []
  }

  return {
    token,
    profile,
    permissions,
    role,
    isLogin,
    login,
    fetchMe,
    loadPermissions,
    hasRole,
    logout
  }
})
