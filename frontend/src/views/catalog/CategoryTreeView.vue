<template>
  <div class="page-container">
    <el-card shadow="never">
      <div class="flex-between mb-16">
        <span class="card-title">服务分类树</span>
        <el-button type="primary" @click="openDialog(null, null)">新建顶级分类</el-button>
      </div>
      <el-tree
        v-loading="loading"
        :data="store.categories"
        node-key="id"
        default-expand-all
        :props="{ label: 'name', children: 'children' }"
      >
        <template #default="{ data }">
          <span class="tree-node">
            <span>{{ data.name }}</span>
            <span class="tree-actions">
              <el-button text type="primary" size="small" @click.stop="openDialog(data, null)">新增子级</el-button>
              <el-button text type="primary" size="small" @click.stop="openDialog(null, data)">编辑</el-button>
              <el-button text type="danger" size="small" @click.stop="removeCategory(data)">删除</el-button>
            </span>
          </span>
        </template>
      </el-tree>
      <el-empty v-if="!loading && !store.categories.length" description="暂无分类" />
    </el-card>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="420px">
      <el-form label-width="90px">
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="分类名称" />
        </el-form-item>
        <el-form-item label="父级分类">
          <el-select v-model="form.parent_id" placeholder="顶级分类" clearable style="width: 100%">
            <el-option v-for="opt in parentOptions" :key="opt.id" :label="opt.name" :value="opt.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort_order" :min="0" />
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
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useCatalogStore } from '@/stores/catalog'
import type { ServiceCategory, ServiceCategoryNode } from '@/types/catalog'

const store = useCatalogStore()

const loading = ref(false)
const dialogVisible = ref(false)
const submitting = ref(false)
const editingId = ref<number | null>(null)
const form = reactive<{ name: string; parent_id: number | null; sort_order: number }>({
  name: '',
  parent_id: null,
  sort_order: 0
})

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

const parentOptions = computed(() => flatten(store.categories))

const dialogTitle = computed(() => (editingId.value ? '编辑分类' : '新建分类'))

function openDialog(parent: ServiceCategory | null, edit: ServiceCategory | null): void {
  if (edit) {
    editingId.value = edit.id
    form.name = edit.name
    form.parent_id = edit.parent_id
    form.sort_order = edit.sort_order
  } else {
    editingId.value = null
    form.name = ''
    form.parent_id = parent?.id ?? null
    form.sort_order = 0
  }
  dialogVisible.value = true
}

async function submit(): Promise<void> {
  if (!form.name) {
    ElMessage.warning('请填写分类名称')
    return
  }
  submitting.value = true
  try {
    await store.saveCategory(editingId.value, { ...form })
    ElMessage.success('已保存')
    dialogVisible.value = false
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

async function removeCategory(node: ServiceCategory): Promise<void> {
  try {
    await ElMessageBox.confirm(`确认删除分类「${node.name}」？含子分类或服务项时将被拒绝。`, '危险操作', {
      type: 'warning'
    })
  } catch {
    return
  }
  try {
    await store.removeCategory(node.id)
    ElMessage.success('已删除')
  } catch {
    /* 409 提示由拦截器展示 */
  }
}

async function load(): Promise<void> {
  loading.value = true
  try {
    await store.fetchTree()
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void load()
})
</script>

<style scoped>
.tree-node {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding-right: 8px;
}
.tree-actions {
  display: none;
}
.tree-node:hover .tree-actions {
  display: inline-flex;
  gap: 4px;
}
</style>
