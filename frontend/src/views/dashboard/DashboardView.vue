<template>
  <div class="page-container">
    <div class="welcome">
      <span class="welcome-name">{{ userStore.profile?.display_name || '用户' }}</span>
      <span class="text-muted">，欢迎使用 ITSM-CORE 工作台</span>
    </div>

    <el-row :gutter="16" class="mt-16">
      <el-col v-for="card in cards" :key="card.key" :xs="24" :sm="12" :md="6">
        <el-card shadow="hover" class="stat-card" @click="card.to && router.push(card.to)">
          <div class="stat-label">{{ card.label }}</div>
          <div class="stat-value" :class="card.tone">{{ card.value }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16" class="mt-16">
      <el-col :xs="24" :md="12">
        <el-card shadow="never">
          <template #header><span class="card-title">快捷入口</span></template>
          <div class="quick-links">
            <el-button @click="router.push('/tickets/new')">提交工单</el-button>
            <el-button @click="router.push('/catalog')">自助服务目录</el-button>
            <el-button v-if="showServiceDesk" @click="router.push('/incidents/new')">上报事件</el-button>
            <el-button v-if="showChange" @click="router.push('/changes/new')">提交变更</el-button>
          </div>
        </el-card>
      </el-col>
      <el-col :xs="24" :md="12">
        <el-card shadow="never">
          <template #header><span class="card-title">我的角色权限</span></template>
          <el-space wrap>
            <el-tag v-for="perm in userStore.permissions" :key="perm" type="info" size="small">
              {{ perm }}
            </el-tag>
            <span v-if="!userStore.permissions.length" class="text-muted">无权限点</span>
          </el-space>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { fetchTickets } from '@/api/ticket'
import { fetchChanges } from '@/api/change'
import type { Role } from '@/types/common'

interface StatCard {
  key: string
  label: string
  value: number
  tone: string
  to?: string
}

const router = useRouter()
const userStore = useUserStore()

const cards = ref<StatCard[]>([
  { key: 'my', label: '待我处理工单', value: 0, tone: '', to: '/tickets' },
  { key: 'progress', label: '处理中工单', value: 0, tone: 'tone-warning', to: '/tickets' },
  { key: 'breached', label: 'SLA 超期工单', value: 0, tone: 'tone-danger', to: '/tickets' },
  { key: 'approval', label: '待审批变更', value: 0, tone: 'tone-primary', to: '/changes' }
])

const role = computed<Role | ''>(() => userStore.role as Role | '')
const showServiceDesk = computed(() =>
  ['agent', 'resolver', 'problem_manager', 'change_manager', 'admin'].includes(role.value)
)
const showChange = computed(() => Boolean(role.value))

function setCard(key: string, value: number): void {
  const card = cards.value.find((item) => item.key === key)
  if (card) card.value = value
}

async function loadStats(): Promise<void> {
  const me = userStore.profile?.id
  try {
    if (me) {
      const res = await fetchTickets({ assignee_id: me, page: 1, page_size: 1 })
      setCard('my', res.total)
    }
  } catch {
    /* ignore */
  }
  try {
    const res = await fetchTickets({ status: 'in_progress', page: 1, page_size: 1 })
    setCard('progress', res.total)
  } catch {
    /* ignore */
  }
  try {
    const res = await fetchTickets({ sla_status: 'breached', page: 1, page_size: 1 })
    setCard('breached', res.total)
  } catch {
    /* ignore */
  }
  try {
    const res = await fetchChanges({ status: 'pending_approval', page: 1, page_size: 1 })
    setCard('approval', res.total)
  } catch {
    /* ignore */
  }
}

onMounted(() => {
  void loadStats()
})
</script>

<style scoped>
.welcome {
  font-size: 16px;
}
.welcome-name {
  font-weight: 700;
  color: #2b6cb0;
}
.stat-card {
  cursor: pointer;
}
.stat-label {
  color: #909399;
  font-size: 13px;
}
.stat-value {
  font-size: 30px;
  font-weight: 700;
  margin-top: 8px;
  color: #303133;
}
.tone-warning {
  color: #e6a23c;
}
.tone-danger {
  color: #f56c6c;
}
.tone-primary {
  color: #409eff;
}
.quick-links {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
</style>
