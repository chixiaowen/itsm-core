<template>
  <div class="page-container">
    <el-page-header :content="isEdit ? '编辑工单' : '新建工单'" @back="router.back()" />
    <el-card shadow="never" class="mt-16">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" style="max-width: 720px">
        <el-form-item label="标题" prop="title">
          <el-input v-model="form.title" placeholder="简要描述问题" clearable />
        </el-form-item>
        <el-form-item label="分类" prop="category_id">
          <el-select v-model="form.category_id" placeholder="选择工单分类" clearable style="width: 100%">
            <el-option v-for="opt in categoryOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="优先级" prop="priority">
          <el-select v-model="form.priority" placeholder="选择优先级" style="width: 200px">
            <el-option v-for="opt in PRIORITY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="描述" prop="description">
          <el-input v-model="form.description" type="textarea" :rows="5" placeholder="详细描述问题现象与影响" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="submitting" @click="submit">
            {{ isEdit ? '保存' : '提交工单' }}
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
import { useTicketStore } from '@/stores/ticket'
import { PRIORITY_OPTIONS, type Priority } from '@/types/common'
import type { CreateTicketReq, TicketCategoryNode } from '@/types/ticket'

const route = useRoute()
const router = useRouter()
const store = useTicketStore()

const formRef = ref<FormInstance>()
const submitting = ref(false)
const ticketId = computed(() => (route.params.id ? Number(route.params.id) : null))
const isEdit = computed(() => ticketId.value !== null)

const form = reactive<CreateTicketReq>({
  title: '',
  description: '',
  category_id: null,
  priority: 'P4'
})

const rules: FormRules = {
  title: [{ required: true, message: '请输入标题', trigger: 'blur' }]
}

interface FlatCategory {
  label: string
  value: number
}

/** 扁平化分类树为带缩进的选项 */
function flatten(nodes: TicketCategoryNode[], depth = 0, acc: FlatCategory[] = []): FlatCategory[] {
  nodes.forEach((node) => {
    acc.push({ label: `${'　'.repeat(depth)}${node.name}`, value: node.id })
    if (node.children?.length) flatten(node.children, depth + 1, acc)
  })
  return acc
}

const categoryOptions = ref<FlatCategory[]>([])

async function loadCategories(): Promise<void> {
  const tree = await store.fetchCategories()
  categoryOptions.value = flatten(tree)
}

async function loadTicket(): Promise<void> {
  if (!ticketId.value) return
  const data = await store.fetchDetail(ticketId.value)
  form.title = data.title
  form.description = data.description
  form.category_id = data.category_id
  form.priority = data.priority
}

async function submit(): Promise<void> {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    let saved
    if (isEdit.value && ticketId.value) {
      saved = await store.update(ticketId.value, {
        title: form.title,
        description: form.description,
        category_id: form.category_id,
        priority: form.priority as Priority
      })
      ElMessage.success('保存成功')
    } else {
      saved = await store.create(form)
      ElMessage.success(`工单已创建：${saved.code}`)
    }
    await router.push(`/tickets/${saved.id}`)
  } catch {
    // 错误已由拦截器提示
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  await loadCategories()
  if (isEdit.value) await loadTicket()
})
</script>
