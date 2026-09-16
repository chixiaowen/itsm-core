<template>
  <div class="page-container">
    <el-page-header :content="isEdit ? '编辑服务项' : '新建服务项'" @back="router.back()" />

    <el-card shadow="never" class="mt-16">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px" style="max-width: 820px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="服务项名称" clearable />
        </el-form-item>
        <el-form-item label="分类" prop="category_id">
          <el-select v-model="form.category_id" placeholder="选择分类" style="width: 260px">
            <el-option v-for="opt in categoryOptions" :key="opt.id" :label="opt.name" :value="opt.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="SLA 策略">
          <el-select v-model="form.sla_policy_id" placeholder="选择 SLA 策略" clearable style="width: 260px">
            <el-option v-for="policy in slaOptions" :key="policy.id" :label="policy.name" :value="policy.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="默认优先级">
          <el-select v-model="form.default_priority" style="width: 200px">
            <el-option v-for="opt in PRIORITY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="需审批">
          <el-switch v-model="form.requires_approval" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="3" placeholder="服务项说明" />
        </el-form-item>

        <el-divider content-position="left">表单字段设计器</el-divider>

        <div v-for="(field, index) in designerFields" :key="index" class="designer-row">
          <el-input v-model="field.label" placeholder="字段标题" style="width: 160px" />
          <el-input v-model="field.name" placeholder="字段名(英文)" style="width: 150px" />
          <el-select v-model="field.type" style="width: 130px">
            <el-option v-for="opt in FORM_FIELD_TYPE_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
          <el-checkbox v-model="field.required">必填</el-checkbox>
          <el-input
            v-if="field.type === 'select'"
            v-model="field.optionsText"
            placeholder="选项，逗号分隔"
            style="width: 200px"
          />
          <el-button text type="danger" @click="removeField(index)">删除</el-button>
        </div>
        <el-button class="mt-8" @click="addField">+ 添加字段</el-button>

        <el-form-item class="mt-16">
          <el-button type="primary" :loading="submitting" @click="submit">
            {{ isEdit ? '保存' : '创建' }}
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
import { useCatalogStore } from '@/stores/catalog'
import { fetchSlaPolicies } from '@/api/platform'
import { PRIORITY_OPTIONS, type Priority } from '@/types/common'
import type { SLAPolicy } from '@/types/platform'
import {
  FORM_FIELD_TYPE_OPTIONS,
  parseFormSchema,
  type FormField,
  type FormFieldType,
  type SaveServiceItemReq,
  type ServiceCategoryNode
} from '@/types/catalog'

interface DesignerField {
  name: string
  label: string
  type: FormFieldType
  required: boolean
  optionsText: string
}

const route = useRoute()
const router = useRouter()
const store = useCatalogStore()

const formRef = ref<FormInstance>()
const submitting = ref(false)
const itemId = computed(() => (route.params.id ? Number(route.params.id) : null))
const isEdit = computed(() => itemId.value !== null)

const form = reactive<{
  name: string
  description: string
  category_id: number | null
  sla_policy_id: number | null
  default_priority: Priority
  requires_approval: boolean
}>({
  name: '',
  description: '',
  category_id: null,
  sla_policy_id: null,
  default_priority: 'P4',
  requires_approval: false
})

const designerFields = ref<DesignerField[]>([])
const slaOptions = ref<SLAPolicy[]>([])

const rules: FormRules = {
  name: [{ required: true, message: '请输入服务项名称', trigger: 'blur' }],
  category_id: [{ required: true, message: '请选择分类', trigger: 'change' }]
}

interface FlatCategory {
  id: number
  name: string
}

function flatten(nodes: ServiceCategoryNode[], acc: FlatCategory[] = []): FlatCategory[] {
  nodes.forEach((node) => {
    acc.push({ id: node.id, name: node.name })
    if (node.children?.length) flatten(node.children, acc)
  })
  return acc
}

const categoryOptions = computed(() => flatten(store.categories))

function addField(): void {
  designerFields.value.push({ name: '', label: '', type: 'text', required: false, optionsText: '' })
}

function removeField(index: number): void {
  designerFields.value.splice(index, 1)
}

function buildSchema(): string {
  const fields: FormField[] = designerFields.value
    .filter((field) => field.name && field.label)
    .map((field, index) => ({
      name: field.name,
      label: field.label,
      type: field.type,
      required: field.required,
      sort: index,
      options:
        field.type === 'select'
          ? field.optionsText
              .split(',')
              .map((item) => item.trim())
              .filter(Boolean)
          : undefined
    }))
  return JSON.stringify(fields)
}

async function loadSla(): Promise<void> {
  try {
    slaOptions.value = await fetchSlaPolicies()
  } catch {
    slaOptions.value = []
  }
}

async function loadItem(): Promise<void> {
  if (!itemId.value) return
  const item = await store.fetchItem(itemId.value)
  form.name = item.name
  form.description = item.description
  form.category_id = item.category_id
  form.sla_policy_id = item.sla_policy_id
  form.default_priority = item.default_priority
  form.requires_approval = item.requires_approval
  designerFields.value = parseFormSchema(item.form_schema).map((field) => ({
    name: field.name,
    label: field.label,
    type: field.type,
    required: field.required,
    optionsText: (field.options || []).join(',')
  }))
}

async function submit(): Promise<void> {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  const payload: SaveServiceItemReq = {
    name: form.name,
    description: form.description,
    category_id: form.category_id as number,
    sla_policy_id: form.sla_policy_id,
    default_priority: form.default_priority,
    requires_approval: form.requires_approval,
    form_schema: buildSchema()
  }
  submitting.value = true
  try {
    const saved = await store.saveItem(itemId.value, payload)
    ElMessage.success('已保存')
    await router.push(`/admin/service-items/${saved.id}`)
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  await store.fetchTree()
  await loadSla()
  if (isEdit.value) await loadItem()
})
</script>

<style scoped>
.designer-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
  flex-wrap: wrap;
}
</style>
