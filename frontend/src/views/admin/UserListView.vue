<template>
  <PageTable
    :total="total"
    :page="query.page"
    :page-size="query.page_size"
    :loading="loading"
    @update:page="(v: number) => load({ page: v })"
    @update:page-size="(v: number) => load({ page: 1, page_size: v })"
  >
    <template #filters>
      <el-select v-model="filters.role" placeholder="角色" clearable style="width: 160px">
        <el-option v-for="opt in roleOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-input v-model="filters.keyword" placeholder="用户名/姓名" clearable style="width: 200px" @keyup.enter="search" />
      <el-button type="primary" @click="search">查询</el-button>
      <el-button @click="reset">重置</el-button>
    </template>

    <template #toolbar>
      <span class="text-muted">共 {{ total }} 个用户</span>
      <el-button type="primary" @click="openDialog(null)">新建用户</el-button>
    </template>

    <el-table v-loading="loading" :data="users" style="width: 100%">
      <el-table-column prop="username" label="用户名" width="160" fixed />
      <el-table-column prop="display_name" label="姓名" min-width="140" />
      <el-table-column label="角色" width="140">
        <template #default="{ row }">{{ ROLE_LABELS[row.role as Role] }}</template>
      </el-table-column>
      <el-table-column prop="email" label="邮箱" min-width="180" />
      <el-table-column label="状态" width="100">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column label="创建时间" width="160">
        <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="160" fixed="right">
        <template #default="{ row }">
          <el-button text type="primary" @click="openDialog(row as User)">编辑</el-button>
          <el-button text type="danger" @click="removeUser(row as User)">删除</el-button>
        </template>
      </el-table-column>
      <template #empty><el-empty description="暂无用户" /></template>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑用户' : '新建用户'" width="460px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" :disabled="Boolean(editingId)" placeholder="登录用户名" />
        </el-form-item>
        <el-form-item label="姓名" prop="display_name">
          <el-input v-model="form.display_name" placeholder="显示名称" />
        </el-form-item>
        <el-form-item label="角色" prop="role">
          <el-select v-model="form.role" style="width: 100%">
            <el-option v-for="opt in roleOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="form.email" placeholder="邮箱（可选）" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" type="password" show-password :placeholder="editingId ? '留空则不修改' : '初始密码'" />
        </el-form-item>
        <el-form-item v-if="editingId" label="状态">
          <el-select v-model="form.status" style="width: 100%">
            <el-option label="启用" value="active" />
            <el-option label="禁用" value="disabled" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </PageTable>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage, ElMessageBox } from 'element-plus'
import PageTable from '@/components/PageTable.vue'
import StatusTag from '@/components/StatusTag.vue'
import {
  createUser,
  deleteUser,
  fetchUsers,
  updateUser
} from '@/api/platform'
import { formatDateTime, ROLE_LABELS, type Role } from '@/types/common'
import type { User, UserStatus } from '@/types/platform'

const users = ref<User[]>([])
const total = ref(0)
const loading = ref(false)
const submitting = ref(false)
const query = reactive({ page: 1, page_size: 20 })
const filters = reactive({ role: '', keyword: '' })

const dialogVisible = ref(false)
const editingId = ref<number | null>(null)
const formRef = ref<FormInstance>()

const roleOptions = (Object.keys(ROLE_LABELS) as Role[]).map((role) => ({
  label: ROLE_LABELS[role],
  value: role
}))

const form = reactive<{
  username: string
  display_name: string
  role: Role
  email: string
  password: string
  status: UserStatus
}>({
  username: '',
  display_name: '',
  role: 'requestor',
  email: '',
  password: '',
  status: 'active'
})

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  display_name: [{ required: true, message: '请输入姓名', trigger: 'blur' }],
  role: [{ required: true, message: '请选择角色', trigger: 'change' }]
}

async function load(extra?: Partial<typeof query>): Promise<void> {
  if (extra) Object.assign(query, extra)
  loading.value = true
  try {
    const res = await fetchUsers({
      page: query.page,
      page_size: query.page_size,
      role: filters.role || undefined,
      keyword: filters.keyword || undefined
    })
    users.value = res.items
    total.value = res.total
  } finally {
    loading.value = false
  }
}

function search(): void {
  load({ page: 1 })
}

function reset(): void {
  filters.role = ''
  filters.keyword = ''
  load({ page: 1 })
}

function openDialog(user: User | null): void {
  if (user) {
    editingId.value = user.id
    form.username = user.username
    form.display_name = user.display_name
    form.role = user.role
    form.email = user.email
    form.password = ''
    form.status = user.status
  } else {
    editingId.value = null
    form.username = ''
    form.display_name = ''
    form.role = 'requestor'
    form.email = ''
    form.password = ''
    form.status = 'active'
  }
  dialogVisible.value = true
}

async function submit(): Promise<void> {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    if (editingId.value) {
      await updateUser(editingId.value, {
        display_name: form.display_name,
        role: form.role,
        status: form.status,
        password: form.password || undefined
      })
    } else {
      await createUser({
        username: form.username,
        display_name: form.display_name,
        role: form.role,
        email: form.email,
        password: form.password || 'admin123'
      })
    }
    ElMessage.success('已保存')
    dialogVisible.value = false
    await load()
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

async function removeUser(user: User): Promise<void> {
  try {
    await ElMessageBox.confirm(`确认删除用户「${user.display_name}」？`, '危险操作', { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteUser(user.id)
    ElMessage.success('已删除')
    await load()
  } catch {
    /* handled by interceptor */
  }
}

onMounted(() => {
  void load()
})
</script>
