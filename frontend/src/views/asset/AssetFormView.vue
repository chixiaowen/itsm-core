<template>
  <div class="page-container">
    <el-page-header :content="isEdit ? '编辑资产' : '新建资产'" @back="router.back()" />
    <el-card shadow="never" class="mt-16">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" style="max-width: 720px">
        <el-form-item label="资产编号" prop="asset_no">
          <el-input v-model="form.asset_no" placeholder="唯一资产编号" clearable />
        </el-form-item>
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="资产名称" clearable />
        </el-form-item>
        <el-form-item label="类别" prop="category">
          <el-select v-model="form.category" style="width: 220px">
            <el-option v-for="opt in ASSET_CATEGORY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="供应商">
          <el-input v-model="form.vendor" placeholder="供应商" clearable />
        </el-form-item>
        <el-form-item label="采购日期">
          <el-date-picker v-model="form.purchase_date" type="date" value-format="YYYY-MM-DD" placeholder="选择日期" style="width: 220px" />
        </el-form-item>
        <el-form-item label="保修到期">
          <el-date-picker v-model="form.warranty_end" type="date" value-format="YYYY-MM-DD" placeholder="选择日期" style="width: 220px" />
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
import { useAssetStore } from '@/stores/asset'
import { ASSET_CATEGORY_OPTIONS, type SaveAssetReq } from '@/types/asset'

const route = useRoute()
const router = useRouter()
const store = useAssetStore()

const formRef = ref<FormInstance>()
const submitting = ref(false)
const assetId = computed(() => (route.params.id ? Number(route.params.id) : null))
const isEdit = computed(() => assetId.value !== null)

const form = reactive<SaveAssetReq>({
  asset_no: '',
  name: '',
  category: 'server',
  vendor: '',
  purchase_date: null,
  warranty_end: null
})

const rules: FormRules = {
  asset_no: [{ required: true, message: '请输入资产编号', trigger: 'blur' }],
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  category: [{ required: true, message: '请选择类别', trigger: 'change' }]
}

async function loadAsset(): Promise<void> {
  if (!assetId.value) return
  const asset = await store.fetchDetail(assetId.value)
  form.asset_no = asset.asset_no
  form.name = asset.name
  form.category = asset.category
  form.vendor = asset.vendor
  form.purchase_date = asset.purchase_date
  form.warranty_end = asset.warranty_end
}

async function submit(): Promise<void> {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    const saved = isEdit.value && assetId.value
      ? await store.update(assetId.value, { ...form })
      : await store.create({ ...form })
    ElMessage.success('已保存')
    await router.push(`/cmdb/assets/${saved.id}`)
  } catch {
    /* handled by interceptor */
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  if (isEdit.value) void loadAsset()
})
</script>
