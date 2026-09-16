<template>
  <div class="page-container">
    <el-page-header :content="isEdit ? '编辑 CI' : '新建 CI'" @back="router.back()" />
    <el-card shadow="never" class="mt-16">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" style="max-width: 720px">
        <el-form-item label="编码" prop="code">
          <el-input v-model="form.code" placeholder="唯一编码" clearable />
        </el-form-item>
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="CI 名称" clearable />
        </el-form-item>
        <el-form-item label="类型" prop="ci_type">
          <el-select v-model="form.ci_type" style="width: 220px">
            <el-option v-for="opt in CI_TYPE_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态" prop="status">
          <el-select v-model="form.status" style="width: 220px">
            <el-option v-for="opt in CI_STATUS_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="负责人">
          <UserSelect v-model="form.owner_id" placeholder="请选择负责人" style="width: 220px" />
        </el-form-item>
        <el-form-item label="自定义属性">
          <div class="attr-editor">
            <div v-for="(attr, index) in attrs" :key="index" class="attr-row">
              <el-input v-model="attr.key" placeholder="属性名" style="width: 180px" />
              <el-input v-model="attr.value" placeholder="属性值" style="width: 240px" />
              <el-button text type="danger" @click="attrs.splice(index, 1)">删除</el-button>
            </div>
            <el-button size="small" class="mt-8" @click="attrs.push({ key: '', value: '' })">+ 添加属性</el-button>
          </div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="submitting" @click="submit">{{ isEdit ? '保存' : '创建' }}</el-button>
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
import UserSelect from '@/components/UserSelect.vue'
import { useCmdbStore } from '@/stores/cmdb'
import { CI_STATUS_OPTIONS, CI_TYPE_OPTIONS, type CiStatus, type CiType, type SaveCiReq } from '@/types/cmdb'

const route = useRoute()
const router = useRouter()
const store = useCmdbStore()

const formRef = ref<FormInstance>()
const submitting = ref(false)
const ciId = computed(() => (route.params.id ? Number(route.params.id) : null))
const isEdit = computed(() => ciId.value !== null)

const form = reactive<{
  code: string
  name: string
  ci_type: CiType
  status: CiStatus
  owner_id?: number
}>({
  code: '',
  name: '',
  ci_type: 'server',
  status: 'planned',
  owner_id: undefined
})

const attrs = ref<{ key: string; value: string }[]>([])

const rules: FormRules = {
  code: [{ required: true, message: '请输入编码', trigger: 'blur' }],
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  ci_type: [{ required: true, message: '请选择类型', trigger: 'change' }]
}

function buildAttrs(): string {
  const obj: Record<string, string> = {}
  attrs.value.forEach((attr) => {
    if (attr.key) obj[attr.key] = attr.value
  })
  return JSON.stringify(obj)
}

async function loadCi(): Promise<void> {
  if (!ciId.value) return
  const ci = await store.fetchDetail(ciId.value)
  form.code = ci.code
  form.name = ci.name
  form.ci_type = ci.ci_type
  form.status = ci.status
  form.owner_id = ci.owner_id ?? undefined
  try {
    const parsed = JSON.parse(ci.attrs || '{}') as Record<string, string>
    attrs.value = Object.entries(parsed).map(([key, value]) => ({ key, value: String(value) }))
  } catch {
    attrs.value = []
  }
}

async function submit(): Promise<void> {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  const payload: SaveCiReq = {
    code: form.code,
    name: form.name,
    ci_type: form.ci_type,
    status: form.status,
    owner_id: form.owner_id ?? null,
    attrs: buildAttrs()
  }
  submitting.value = true
  try {
    const saved = await store.saveCi(ciId.value, payload)
    ElMessage.success('已保存')
    await router.push(`/cmdb/cis/${saved.id}`)
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  if (isEdit.value) void loadCi()
})
</script>

<style scoped>
.attr-editor {
  width: 100%;
}
.attr-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
}
</style>
