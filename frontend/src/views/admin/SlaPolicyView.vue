<template>
  <div class="page-container">
    <el-card shadow="never">
      <div class="flex-between mb-16">
        <span class="card-title">SLA 策略配置（按优先级，7×24 自然时间）</span>
        <el-button type="primary" @click="openDialog(null)">新建策略</el-button>
      </div>
      <el-table v-loading="loading" :data="policies" style="width: 100%">
        <el-table-column prop="name" label="策略名称" min-width="160" />
        <el-table-column label="优先级" width="120">
          <template #default="{ row }"><PriorityTag :priority="row.priority" /></template>
        </el-table-column>
        <el-table-column label="响应目标(分钟)" width="150">
          <template #default="{ row }">{{ row.response_minutes }}</template>
        </el-table-column>
        <el-table-column label="解决目标(分钟)" width="150">
          <template #default="{ row }">{{ row.resolve_minutes }}</template>
        </el-table-column>
        <el-table-column label="挂起暂停" width="110">
          <template #default="{ row }">
            <el-tag :type="row.pause_on_pending ? 'success' : 'info'" size="small">
              {{ row.pause_on_pending ? '开启' : '关闭' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button text type="primary" @click="openDialog(row as SLAPolicy)">编辑</el-button>
            <el-button text type="danger" @click="removePolicy(row as SLAPolicy)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty><el-empty description="暂无 SLA 策略" /></template>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑策略' : '新建策略'" width="460px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px">
        <el-form-item label="策略名称" prop="name">
          <el-input v-model="form.name" placeholder="如：P1 紧急策略" />
        </el-form-item>
        <el-form-item label="优先级" prop="priority">
          <el-select v-model="form.priority" style="width: 100%">
            <el-option v-for="opt in PRIORITY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="响应目标(分钟)" prop="response_minutes">
          <el-input-number v-model="form.response_minutes" :min="1" :controls="false" style="width: 100%" />
        </el-form-item>
        <el-form-item label="解决目标(分钟)" prop="resolve_minutes">
          <el-input-number v-model="form.resolve_minutes" :min="1" :controls="false" style="width: 100%" />
        </el-form-item>
        <el-form-item label="挂起暂停计时">
          <el-switch v-model="form.pause_on_pending" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage, ElMessageBox } from 'element-plus'
import PriorityTag from '@/components/PriorityTag.vue'
import { createSlaPolicy, deleteSlaPolicy, fetchSlaPolicies, updateSlaPolicy } from '@/api/platform'
import { PRIORITY_OPTIONS, type Priority } from '@/types/common'
import type { SLAPolicy } from '@/types/platform'

const policies = ref<SLAPolicy[]>([])
const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const editingId = ref<number | null>(null)
const formRef = ref<FormInstance>()

const form = reactive<{
  name: string
  priority: Priority
  response_minutes: number
  resolve_minutes: number
  pause_on_pending: boolean
}>({
  name: '',
  priority: 'P3',
  response_minutes: 120,
  resolve_minutes: 1440,
  pause_on_pending: true
})

const rules: FormRules = {
  name: [{ required: true, message: '请输入策略名称', trigger: 'blur' }],
  priority: [{ required: true, message: '请选择优先级', trigger: 'change' }],
  response_minutes: [{ required: true, message: '请输入响应目标', trigger: 'blur' }],
  resolve_minutes: [{ required: true, message: '请输入解决目标', trigger: 'blur' }]
}

async function load(): Promise<void> {
  loading.value = true
  try {
    policies.value = await fetchSlaPolicies()
  } finally {
    loading.value = false
  }
}

function openDialog(policy: SLAPolicy | null): void {
  if (policy) {
    editingId.value = policy.id
    form.name = policy.name
    form.priority = policy.priority
    form.response_minutes = policy.response_minutes
    form.resolve_minutes = policy.resolve_minutes
    form.pause_on_pending = policy.pause_on_pending
  } else {
    editingId.value = null
    form.name = ''
    form.priority = 'P3'
    form.response_minutes = 120
    form.resolve_minutes = 1440
    form.pause_on_pending = true
  }
  dialogVisible.value = true
}

async function submit(): Promise<void> {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    if (editingId.value) await updateSlaPolicy(editingId.value, { ...form })
    else await createSlaPolicy({ ...form })
    ElMessage.success('已保存')
    dialogVisible.value = false
    await load()
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

async function removePolicy(policy: SLAPolicy): Promise<void> {
  try {
    await ElMessageBox.confirm(`确认删除策略「${policy.name}」？`, '危险操作', { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteSlaPolicy(policy.id)
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
