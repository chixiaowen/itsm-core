/**
 * 路由表 + 角色守卫（ARCHITECTURE §14.2）。
 * - /login 免鉴权
 * - 其余路由经 meta.roles 控制访问
 * - 无 token -> /login；角色不符 -> /403
 */
import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'
import DefaultLayout from '@/layouts/DefaultLayout.vue'
import { getToken } from '@/api/http'
import { useUserStore } from '@/stores/user'
import type { Role } from '@/types/common'

declare module 'vue-router' {
  interface RouteMeta {
    public?: boolean
    roles?: Role[]
    title?: string
    breadcrumb?: string[]
  }
}

const ALL_ROLES: Role[] = [
  'requestor',
  'agent',
  'resolver',
  'problem_manager',
  'change_manager',
  'cmdb_manager',
  'admin'
]

const SERVICE_DESK_ROLES: Role[] = ['agent', 'resolver', 'problem_manager', 'change_manager', 'admin']
const PROBLEM_ROLES: Role[] = ['problem_manager', 'admin']
const CMDB_ROLES: Role[] = ['cmdb_manager', 'admin']
const ADMIN_ONLY: Role[] = ['admin']

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/login/LoginView.vue'),
    meta: { public: true, title: '登录' }
  },
  {
    path: '/',
    component: DefaultLayout,
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'dashboard',
        component: () => import('@/views/dashboard/DashboardView.vue'),
        meta: { title: '工作台', roles: ALL_ROLES, breadcrumb: ['工作台'] }
      },
      // ---------- 工单 ----------
      {
        path: 'tickets',
        name: 'ticket-list',
        component: () => import('@/views/ticket/TicketListView.vue'),
        meta: { title: '工单列表', roles: ALL_ROLES, breadcrumb: ['服务台', '工单列表'] }
      },
      {
        path: 'tickets/new',
        name: 'ticket-create',
        component: () => import('@/views/ticket/TicketFormView.vue'),
        meta: { title: '新建工单', roles: ALL_ROLES, breadcrumb: ['服务台', '新建工单'] }
      },
      {
        path: 'tickets/:id',
        name: 'ticket-detail',
        component: () => import('@/views/ticket/TicketDetailView.vue'),
        meta: { title: '工单详情', roles: ALL_ROLES, breadcrumb: ['服务台', '工单详情'] }
      },
      {
        path: 'tickets/:id/edit',
        name: 'ticket-edit',
        component: () => import('@/views/ticket/TicketFormView.vue'),
        meta: { title: '编辑工单', roles: ALL_ROLES, breadcrumb: ['服务台', '编辑工单'] }
      },
      // ---------- 事件 ----------
      {
        path: 'incidents',
        name: 'incident-list',
        component: () => import('@/views/incident/IncidentListView.vue'),
        meta: { title: '事件列表', roles: SERVICE_DESK_ROLES, breadcrumb: ['服务台', '事件列表'] }
      },
      {
        path: 'incidents/new',
        name: 'incident-create',
        component: () => import('@/views/incident/IncidentFormView.vue'),
        meta: { title: '上报事件', roles: ALL_ROLES, breadcrumb: ['服务台', '上报事件'] }
      },
      {
        path: 'incidents/:id',
        name: 'incident-detail',
        component: () => import('@/views/incident/IncidentDetailView.vue'),
        meta: { title: '事件详情', roles: SERVICE_DESK_ROLES, breadcrumb: ['服务台', '事件详情'] }
      },
      // ---------- 问题 ----------
      {
        path: 'problems',
        name: 'problem-list',
        component: () => import('@/views/problem/ProblemListView.vue'),
        meta: { title: '问题列表', roles: PROBLEM_ROLES, breadcrumb: ['服务台', '问题列表'] }
      },
      {
        path: 'problems/new',
        name: 'problem-create',
        component: () => import('@/views/problem/ProblemFormView.vue'),
        meta: { title: '新建问题', roles: PROBLEM_ROLES, breadcrumb: ['服务台', '新建问题'] }
      },
      {
        path: 'problems/:id',
        name: 'problem-detail',
        component: () => import('@/views/problem/ProblemDetailView.vue'),
        meta: { title: '问题详情', roles: PROBLEM_ROLES, breadcrumb: ['服务台', '问题详情'] }
      },
      // ---------- 变更 ----------
      {
        path: 'changes',
        name: 'change-list',
        component: () => import('@/views/change/ChangeListView.vue'),
        meta: { title: '变更列表', roles: ALL_ROLES, breadcrumb: ['服务台', '变更列表'] }
      },
      {
        path: 'changes/new',
        name: 'change-create',
        component: () => import('@/views/change/ChangeFormView.vue'),
        meta: { title: '变更申请', roles: ALL_ROLES, breadcrumb: ['服务台', '变更申请'] }
      },
      {
        path: 'changes/:id',
        name: 'change-detail',
        component: () => import('@/views/change/ChangeDetailView.vue'),
        meta: { title: '变更详情', roles: ALL_ROLES, breadcrumb: ['服务台', '变更详情'] }
      },
      {
        path: 'changes/:id/edit',
        name: 'change-edit',
        component: () => import('@/views/change/ChangeFormView.vue'),
        meta: { title: '编辑变更', roles: ALL_ROLES, breadcrumb: ['服务台', '编辑变更'] }
      },
      // ---------- 服务目录（自助） ----------
      {
        path: 'catalog',
        name: 'catalog-portal',
        component: () => import('@/views/catalog/ServiceOrderView.vue'),
        meta: { title: '服务目录', roles: ALL_ROLES, breadcrumb: ['自助服务', '服务目录'] }
      },
      {
        path: 'catalog/items/:id',
        name: 'catalog-item-detail',
        component: () => import('@/views/catalog/ServiceItemDetailView.vue'),
        meta: { title: '服务项详情', roles: ALL_ROLES, breadcrumb: ['自助服务', '服务项详情'] }
      },
      // ---------- 服务目录（管理台） ----------
      {
        path: 'admin/categories',
        name: 'category-tree',
        component: () => import('@/views/catalog/CategoryTreeView.vue'),
        meta: { title: '分类管理', roles: ADMIN_ONLY, breadcrumb: ['管理', '分类管理'] }
      },
      {
        path: 'admin/service-items',
        name: 'service-item-list',
        component: () => import('@/views/catalog/ServiceItemListView.vue'),
        meta: { title: '服务项管理', roles: ADMIN_ONLY, breadcrumb: ['管理', '服务项管理'] }
      },
      {
        path: 'admin/service-items/new',
        name: 'service-item-create',
        component: () => import('@/views/catalog/ServiceItemFormView.vue'),
        meta: { title: '新建服务项', roles: ADMIN_ONLY, breadcrumb: ['管理', '新建服务项'] }
      },
      {
        path: 'admin/service-items/:id',
        name: 'service-item-detail',
        component: () => import('@/views/catalog/ServiceItemDetailView.vue'),
        meta: { title: '服务项详情', roles: ADMIN_ONLY, breadcrumb: ['管理', '服务项详情'] }
      },
      {
        path: 'admin/service-items/:id/edit',
        name: 'service-item-edit',
        component: () => import('@/views/catalog/ServiceItemFormView.vue'),
        meta: { title: '编辑服务项', roles: ADMIN_ONLY, breadcrumb: ['管理', '编辑服务项'] }
      },
      // ---------- CMDB ----------
      {
        path: 'cmdb/cis',
        name: 'ci-list',
        component: () => import('@/views/cmdb/CiListView.vue'),
        meta: { title: 'CI 列表', roles: CMDB_ROLES, breadcrumb: ['资产与配置', 'CI 列表'] }
      },
      {
        path: 'cmdb/cis/new',
        name: 'ci-create',
        component: () => import('@/views/cmdb/CiFormView.vue'),
        meta: { title: '新建 CI', roles: CMDB_ROLES, breadcrumb: ['资产与配置', '新建 CI'] }
      },
      {
        path: 'cmdb/cis/:id',
        name: 'ci-detail',
        component: () => import('@/views/cmdb/CiDetailView.vue'),
        meta: { title: 'CI 详情', roles: CMDB_ROLES, breadcrumb: ['资产与配置', 'CI 详情'] }
      },
      {
        path: 'cmdb/cis/:id/edit',
        name: 'ci-edit',
        component: () => import('@/views/cmdb/CiFormView.vue'),
        meta: { title: '编辑 CI', roles: CMDB_ROLES, breadcrumb: ['资产与配置', '编辑 CI'] }
      },
      {
        path: 'cmdb/topology',
        name: 'ci-topology',
        component: () => import('@/views/cmdb/CiTopologyView.vue'),
        meta: { title: 'CI 拓扑', roles: CMDB_ROLES, breadcrumb: ['资产与配置', 'CI 拓扑'] }
      },
      {
        path: 'cmdb/cis/:id/topology',
        name: 'ci-topology-detail',
        component: () => import('@/views/cmdb/CiTopologyView.vue'),
        meta: { title: 'CI 拓扑', roles: CMDB_ROLES, breadcrumb: ['资产与配置', 'CI 拓扑'] }
      },
      {
        path: 'cmdb/assets',
        name: 'asset-list',
        component: () => import('@/views/asset/AssetListView.vue'),
        meta: { title: '资产台账', roles: CMDB_ROLES, breadcrumb: ['资产与配置', '资产台账'] }
      },
      {
        path: 'cmdb/assets/new',
        name: 'asset-create',
        component: () => import('@/views/asset/AssetFormView.vue'),
        meta: { title: '新建资产', roles: CMDB_ROLES, breadcrumb: ['资产与配置', '新建资产'] }
      },
      {
        path: 'cmdb/assets/:id',
        name: 'asset-detail',
        component: () => import('@/views/asset/AssetDetailView.vue'),
        meta: { title: '资产详情', roles: CMDB_ROLES, breadcrumb: ['资产与配置', '资产详情'] }
      },
      {
        path: 'cmdb/assets/:id/edit',
        name: 'asset-edit',
        component: () => import('@/views/asset/AssetFormView.vue'),
        meta: { title: '编辑资产', roles: CMDB_ROLES, breadcrumb: ['资产与配置', '编辑资产'] }
      },
      // ---------- 管理台 ----------
      {
        path: 'admin/users',
        name: 'user-list',
        component: () => import('@/views/admin/UserListView.vue'),
        meta: { title: '用户与角色', roles: ADMIN_ONLY, breadcrumb: ['管理', '用户与角色'] }
      },
      {
        path: 'admin/sla',
        name: 'sla-policy',
        component: () => import('@/views/admin/SlaPolicyView.vue'),
        meta: { title: 'SLA 策略', roles: ADMIN_ONLY, breadcrumb: ['管理', 'SLA 策略'] }
      },
      {
        path: 'admin/audit',
        name: 'audit-log',
        component: () => import('@/views/admin/AuditLogView.vue'),
        meta: { title: '审计日志', roles: ADMIN_ONLY, breadcrumb: ['管理', '审计日志'] }
      }
    ]
  },
  {
    path: '/403',
    name: 'forbidden',
    component: () => import('@/views/error/NotFoundView.vue'),
    props: { code: 403 },
    meta: { title: '无权限' }
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/error/NotFoundView.vue'),
    props: { code: 404 },
    meta: { title: '页面不存在' }
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
  scrollBehavior: () => ({ top: 0 })
})

router.beforeEach(async (to) => {
  const token = getToken()
  const userStore = useUserStore()

  if (to.meta.public) {
    if (token && to.path === '/login') return { path: '/' }
    return true
  }

  if (!token) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }

  if (!userStore.profile) {
    try {
      await userStore.fetchMe()
      await userStore.loadPermissions()
    } catch {
      userStore.logout()
      return { path: '/login' }
    }
  }

  const roles = to.meta.roles
  if (roles && roles.length > 0 && !roles.includes(userStore.role as Role)) {
    return { path: '/403' }
  }
  return true
})

export default router
