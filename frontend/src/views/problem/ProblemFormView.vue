<template>
  <div class="page-container">
    <el-page-header content="新建问题" @back="router.back()" />
    <el-card shadow="never" class="mt-16">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" style="max-width: 720px">
        <el-form-item label="标题" prop="title">
          <el-input v-model="form.title" placeholder="简要描述问题" clearable />
        </el-form-item>
        <el-form-item label="来源" prop="source">
          <el-radio-group v-model="form.source">
            <el-radio value="manual">手动创建</el-radio>
            <el-radio value="aggregate">事件聚合</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.source === 'aggregate'" label="聚合事件">
          <el-select
            v-model="incidentIds"
            multiple
            filterable
            allow-create
            default-first-option
            placeholder="输入或选择事件 ID"
            style="width: 100%"
          >
            <el-option v-for="id in incidentIds" :key="id" :label="`事件 #${id}`" :value="id" />
          </el-select>
          <div class="text-muted mt-8">聚合需选择 ≥1 个事件（PRD REQ-PRB-001）。</div>
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="5" placeholder="描述问题背景与影响" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="submitting" @click="submit">创建问题</el-button>
          <el-button @click="router.back()">取消</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { useProblemStore } from '@/stores/problem'
import type { CreateProblemReq } from '@/types/problem'

const router = useRouter()
const store = useProblemStore()

const formRef = ref<FormInstance>()
const submitting = ref(false)
const incidentIds = ref<number[]>([])

const form = reactive<CreateProblemReq>({
  title: '',
  description: '',
  source: 'manual',
  incident_ids: []
})

const rules: FormRules = {
  title: [{ required: true, message: '请输入标题', trigger: 'blur' }],
  source: [{ required: true, message: '请选择来源', trigger: 'change' }]
}

async function submit(): Promise<void> {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  if (form.source === 'aggregate' && incidentIds.value.length === 0) {
    ElMessage.warning('事件聚合至少需要选择 1 个事件')
    return
  }
  submitting.value = true
  try {
    const payload: CreateProblemReq = {
      title: form.title,
      description: form.description,
      source: form.source,
      incident_ids: form.source === 'aggregate' ? incidentIds.value : []
    }
    const problem = await store.create(payload)
    ElMessage.success(`问题已创建：${problem.code}`)
    await router.push(`/problems/${problem.id}`)
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}
</script>
