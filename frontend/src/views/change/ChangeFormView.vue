<template>
  <div class="page-container">
    <el-page-header :content="isEdit ? '编辑变更' : '变更申请'" @back="router.back()" />
    <el-card shadow="never" class="mt-16">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px" style="max-width: 760px">
        <el-form-item label="标题" prop="title">
          <el-input v-model="form.title" placeholder="变更标题" clearable />
        </el-form-item>
        <el-form-item label="变更类型" prop="change_type">
          <el-select v-model="form.change_type" style="width: 220px" :disabled="typeLocked">
            <el-option v-for="opt in CHANGE_TYPE_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
          <span v-if="typeLocked" class="text-muted" style="margin-left: 8px">进入审批后类型不可修改</span>
        </el-form-item>
        <el-form-item label="风险等级" prop="risk_level">
          <el-select v-model="form.risk_level" style="width: 220px">
            <el-option v-for="opt in RISK_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="3" placeholder="变更背景" />
        </el-form-item>
        <el-form-item label="影响分析">
          <el-input v-model="form.impact_analysis" type="textarea" :rows="3" placeholder="受影响范围与风险评估" />
        </el-form-item>
        <el-form-item label="实施计划">
          <el-input v-model="form.plan" type="textarea" :rows="3" placeholder="实施步骤（提交审批前必填）" />
        </el-form-item>
        <el-form-item label="回滚方案">
          <el-input v-model="form.rollback_plan" type="textarea" :rows="3" placeholder="回滚步骤（提交审批前必填）" />
        </el-form-item>
        <el-form-item label="变更窗口">
          <el-date-picker
            v-model="windowRange"
            type="datetimerange"
            range-separator="至"
            start-placeholder="开始时间"
            end-placeholder="结束时间"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="submitting" @click="submit">
            {{ isEdit ? '保存' : '提交申请' }}
          </el-button>
          <el-button @click="router.back()">取消</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { useChangeStore } from '@/stores/change'
import { CHANGE_TYPE_OPTIONS, RISK_OPTIONS, type CreateChangeReq } from '@/types/change'

const route = useRoute()
const router = useRouter()
const store = useChangeStore()

const formRef = ref<FormInstance>()
const submitting = ref(false)
const windowRange = ref<[string, string] | null>(null)
const typeLocked = ref(false)

const changeId = computed(() => (route.params.id ? Number(route.params.id) : null))
const isEdit = computed(() => changeId.value !== null)

const form = reactive<
  CreateChangeReq & { impact_analysis?: string; plan?: string; rollback_plan?: string }
>({
  title: '',
  description: '',
  change_type: 'normal',
  risk_level: 'low',
  impact_analysis: '',
  plan: '',
  rollback_plan: ''
})

const rules: FormRules = {
  title: [{ required: true, message: '请输入标题', trigger: 'blur' }],
  change_type: [{ required: true, message: '请选择变更类型', trigger: 'change' }],
  risk_level: [{ required: true, message: '请选择风险等级', trigger: 'change' }]
}

async function loadChange(): Promise<void> {
  if (!changeId.value) return
  const data = await store.fetchDetail(changeId.value)
  form.title = data.title
  form.description = data.description
  form.change_type = data.change_type
  form.risk_level = data.risk_level
  form.impact_analysis = data.impact_analysis
  form.plan = data.plan
  form.rollback_plan = data.rollback_plan
  if (data.window_start && data.window_end) {
    windowRange.value = [data.window_start, data.window_end]
  }
  typeLocked.value = data.status !== 'draft'
}

async function submit(): Promise<void> {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    const payload = {
      ...form,
      window_start: windowRange.value?.[0] ?? null,
      window_end: windowRange.value?.[1] ?? null
    }
    if (isEdit.value && changeId.value) {
      await store.update(changeId.value, payload)
      ElMessage.success('保存成功')
      await router.push(`/changes/${changeId.value}`)
    } else {
      const created = await store.create(form)
      ElMessage.success(`变更已创建：${created.code}`)
      await router.push(`/changes/${created.id}`)
    }
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  if (isEdit.value) void loadChange()
})
</script>
