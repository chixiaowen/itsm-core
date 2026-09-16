<template>
  <div class="page-container">
    <el-page-header content="上报事件" @back="router.back()" />
    <el-card shadow="never" class="mt-16">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" style="max-width: 720px">
        <el-form-item label="标题" prop="title">
          <el-input v-model="form.title" placeholder="简要描述故障现象" clearable />
        </el-form-item>
        <el-form-item label="影响度" prop="impact">
          <el-select v-model="form.impact" style="width: 200px">
            <el-option v-for="opt in IMPACT_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="紧急度" prop="urgency">
          <el-select v-model="form.urgency" style="width: 200px">
            <el-option v-for="opt in URGENCY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="预计优先级">
          <PriorityTag :priority="previewPriority" />
          <span class="text-muted" style="margin-left: 8px">（影响度 × 紧急度自动计算）</span>
        </el-form-item>
        <el-form-item label="发生时间" prop="occurred_at">
          <el-date-picker
            v-model="form.occurred_at"
            type="datetime"
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            placeholder="选择发生时间"
            style="width: 240px"
          />
        </el-form-item>
        <el-form-item label="描述" prop="description">
          <el-input v-model="form.description" type="textarea" :rows="5" placeholder="描述影响范围与现象" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="submitting" @click="submit">提交上报</el-button>
          <el-button @click="router.back()">取消</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import PriorityTag from '@/components/PriorityTag.vue'
import { useIncidentStore } from '@/stores/incident'
import {
  computePriority,
  IMPACT_OPTIONS,
  URGENCY_OPTIONS,
  type CreateIncidentReq
} from '@/types/incident'

const router = useRouter()
const store = useIncidentStore()

const formRef = ref<FormInstance>()
const submitting = ref(false)
const form = reactive<CreateIncidentReq>({
  title: '',
  description: '',
  impact: 'medium',
  urgency: 'medium',
  occurred_at: new Date().toISOString()
})

const rules: FormRules = {
  title: [{ required: true, message: '请输入标题', trigger: 'blur' }],
  impact: [{ required: true, message: '请选择影响度', trigger: 'change' }],
  urgency: [{ required: true, message: '请选择紧急度', trigger: 'change' }]
}

const previewPriority = computed(() => computePriority(form.impact, form.urgency))

async function submit(): Promise<void> {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    const incident = await store.report({ ...form })
    ElMessage.success(`事件已上报：${incident.code}`)
    await router.push(`/incidents/${incident.id}`)
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}
</script>
