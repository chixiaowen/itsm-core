<template>
  <el-container class="layout-root">
    <el-aside :width="collapsed ? '64px' : '210px'" class="layout-aside">
      <div class="logo">{{ collapsed ? 'IT' : 'ITSM-CORE' }}</div>
      <el-menu
        :default-active="activeMenu"
        :collapse="collapsed"
        :collapse-transition="false"
        router
        class="layout-menu"
      >
        <template v-for="group in menuGroups" :key="group.title || 'root'">
          <div v-if="group.title && !collapsed" class="menu-group-title">{{ group.title }}</div>
          <el-menu-item v-for="item in group.items" :key="item.index" :index="item.index">
            <span>{{ item.title }}</span>
          </el-menu-item>
        </template>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="layout-header">
        <div class="header-left">
          <el-button text @click="collapsed = !collapsed">{{ collapsed ? '展开' : '收起' }}</el-button>
          <el-breadcrumb separator="/">
            <el-breadcrumb-item v-for="(crumb, idx) in breadcrumbs" :key="idx">{{ crumb }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right">
          <el-badge :value="todoCount" :hidden="todoCount === 0" class="todo-badge">
            <span class="todo-bell">待办</span>
          </el-badge>
          <el-dropdown @command="onUserCommand">
            <span class="user-trigger">
              {{ userStore.profile?.display_name || '未登录' }}
              <span class="text-muted">（{{ roleLabel }}）</span>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="dashboard">我的工作台</el-dropdown-item>
                <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <el-main class="layout-main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ROLE_LABELS, type Role } from '@/types/common'
import { useUserStore } from '@/stores/user'
import { fetchTickets } from '@/api/ticket'
import { fetchChanges } from '@/api/change'

interface MenuItem {
  index: string
  title: string
  roles: Role[]
}

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const collapsed = ref(false)
const todoCount = ref(0)

const ALL: Role[] = ['requestor', 'agent', 'resolver', 'problem_manager', 'change_manager', 'cmdb_manager', 'admin']
const SERVICE_DESK: Role[] = ['agent', 'resolver', 'problem_manager', 'change_manager', 'admin']
const PROBLEM: Role[] = ['problem_manager', 'admin']
const CMDB: Role[] = ['cmdb_manager', 'admin']
const ADMIN: Role[] = ['admin']

const roleLabel = computed(() =>
  userStore.role ? ROLE_LABELS[userStore.role as Role] : '访客'
)

const allGroups: { title: string; items: MenuItem[] }[] = [
  { title: '', items: [{ index: '/dashboard', title: '工作台', roles: ALL }] },
  {
    title: '服务台',
    items: [
      { index: '/tickets', title: '工单列表', roles: ALL },
      { index: '/incidents', title: '事件列表', roles: SERVICE_DESK },
      { index: '/problems', title: '问题列表', roles: PROBLEM },
      { index: '/changes', title: '变更列表', roles: ALL }
    ]
  },
  { title: '自助服务', items: [{ index: '/catalog', title: '服务目录', roles: ALL }] },
  {
    title: '资产与配置',
    items: [
      { index: '/cmdb/cis', title: 'CI 列表', roles: CMDB },
      { index: '/cmdb/topology', title: 'CI 拓扑', roles: CMDB },
      { index: '/cmdb/assets', title: '资产台账', roles: CMDB }
    ]
  },
  {
    title: '管理',
    items: [
      { index: '/admin/service-items', title: '服务项管理', roles: ADMIN },
      { index: '/admin/categories', title: '分类管理', roles: ADMIN },
      { index: '/admin/users', title: '用户与角色', roles: ADMIN },
      { index: '/admin/sla', title: 'SLA 策略', roles: ADMIN },
      { index: '/admin/audit', title: '审计日志', roles: ADMIN }
    ]
  }
]

const menuGroups = computed(() => {
  const role = userStore.role as Role | ''
  return allGroups
    .map((group) => ({
      title: group.title,
      items: group.items.filter((item) => role && item.roles.includes(role))
    }))
    .filter((group) => group.items.length > 0)
})

/** 详情页高亮其列表菜单 */
const activeMenu = computed(() => {
  const path = route.path
  if (path.startsWith('/tickets')) return '/tickets'
  if (path.startsWith('/incidents')) return '/incidents'
  if (path.startsWith('/problems')) return '/problems'
  if (path.startsWith('/changes')) return '/changes'
  if (path.startsWith('/catalog')) return '/catalog'
  if (path.startsWith('/cmdb/assets')) return '/cmdb/assets'
  if (path.startsWith('/cmdb')) return '/cmdb/cis'
  if (path.startsWith('/admin/service-items')) return '/admin/service-items'
  return path
})

const breadcrumbs = computed<string[]>(() => {
  const meta = route.meta.breadcrumb
  if (Array.isArray(meta) && meta.length) return meta
  const title = route.meta.title
  return title ? [title] : []
})

function onUserCommand(command: string): void {
  if (command === 'logout') {
    userStore.logout()
    void router.push({ path: '/login' })
  } else if (command === 'dashboard') {
    void router.push({ path: '/dashboard' })
  }
}

/** 待办角标：SLA 超期工单 + 待审批变更（best-effort，失败静默） */
async function loadTodoCount(): Promise<void> {
  const role = userStore.role as Role | ''
  let count = 0
  try {
    if (['agent', 'resolver', 'admin'].includes(role)) {
      const tickets = await fetchTickets({ sla_status: 'breached', page: 1, page_size: 1 })
      count += tickets.total
    }
  } catch {
    /* ignore */
  }
  try {
    if (['change_manager', 'admin'].includes(role)) {
      const changes = await fetchChanges({ status: 'pending_approval', page: 1, page_size: 1 })
      count += changes.total
    }
  } catch {
    /* ignore */
  }
  todoCount.value = count
}

onMounted(() => {
  void loadTodoCount()
})
</script>

<style scoped>
.layout-root {
  height: 100vh;
}

.layout-aside {
  background-color: #1f2d3d;
  transition: width 0.2s;
  overflow-x: hidden;
}

.logo {
  height: 56px;
  line-height: 56px;
  text-align: center;
  color: #fff;
  font-weight: 700;
  letter-spacing: 1px;
  background-color: #16222f;
}

.layout-menu {
  border-right: none;
  background-color: #1f2d3d;
}

.layout-menu :deep(.el-menu-item) {
  color: #c0c4cc;
}

.layout-menu :deep(.el-menu-item.is-active) {
  color: #fff;
  background-color: #2b6cb0;
}

.layout-menu :deep(.el-menu-item:hover) {
  background-color: #2c3e50;
}

.menu-group-title {
  padding: 12px 20px 4px;
  color: #6b7785;
  font-size: 12px;
}

.layout-header {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  background-color: #fff;
  border-bottom: 1px solid #e4e7ed;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 20px;
}

.todo-bell {
  cursor: default;
  color: #606266;
}

.user-trigger {
  cursor: pointer;
  color: #303133;
  outline: none;
}

.layout-main {
  background-color: #f5f7fa;
  padding: 0;
  overflow-y: auto;
}
</style>
