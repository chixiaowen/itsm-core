<template>
  <el-form ref="formRef" :model="form" :rules="rules" label-width="120px" class="dynamic-form">
    <el-form-item v-for="field in fields" :key="field.name" :label="field.label" :prop="field.name">
      <el-input
        v-if="field.type === 'text'"
        v-model="form[field.name]"
        :placeholder="field.placeholder || `请输入${field.label}`"
        clearable
      />
      <el-input-number
        v-else-if="field.type === 'number'"
        v-model="form[field.name]"
        :controls="false"
        :placeholder="field.placeholder || `请输入${field.label}`"
        style="width: 100%"
      />
      <el-select
        v-else-if="field.type === 'select'"
        v-model="form[field.name]"
        :placeholder="field.placeholder || '请选择'"
        style="width: 100%"
        clearable
      >
        <el-option v-for="opt in field.options || []" :key="opt" :label="opt" :value="opt" />
      </el-select>
      <el-date-picker
        v-else-if="field.type === 'date'"
        v-model="form[field.name]"
        type="date"
        value-format="YYYY-MM-DD"
        :placeholder="field.placeholder || '请选择日期'"
        style="width: 100%"
      />
      <el-input
        v-else-if="field.type === 'textarea'"
        v-model="form[field.name]"
        type="textarea"
        :rows="3"
        :placeholder="field.placeholder || `请输入${field.label}`"
      />
      <el-input v-else v-model="form[field.name]" :placeholder="field.placeholder || ''" />
    </el-form-item>
    <el-empty v-if="!fields.length" description="该服务项未定义表单字段" />
  </el-form>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import type { FormField } from '@/types/catalog'

const props = defineProps<{
  /** 服务项 form_schema 解析后的字段数组 */
  fields: FormField[]
  /** 表单数据（v-model） */
  modelValue: Record<string, string | number | null>
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: Record<string, string | number | null>): void
}>()

const formRef = ref<FormInstance>()
// 内部使用宽松类型以兼容 Element Plus 各输入组件的 v-model 类型约束
const form = reactive<Record<string, any>>({})

// 同步外部 -> 内部
watch(
  () => props.modelValue,
  (value) => {
    Object.keys(form).forEach((key) => delete form[key])
    Object.assign(form, value || {})
    // 补齐未填字段，避免 el-input 拿到 undefined
    props.fields.forEach((field) => {
      if (form[field.name] === undefined) {
        form[field.name] = field.type === 'number' ? null : ''
      }
    })
  },
  { immediate: true, deep: true }
)

// 同步内部 -> 外部
watch(
  form,
  () => {
    emit('update:modelValue', { ...form })
  },
  { deep: true }
)

/** 依据字段定义生成必填校验规则 */
const rules = computed<FormRules>(() => {
  const result: FormRules = {}
  props.fields.forEach((field) => {
    if (!field.required) return
    result[field.name] = [
      {
        required: true,
        message: `请填写${field.label}`,
        trigger: field.type === 'select' || field.type === 'date' ? 'change' : 'blur'
      }
    ]
  })
  return result
})

/** 校验表单，返回是否通过 */
async function validate(): Promise<boolean> {
  if (!formRef.value) return true
  try {
    await formRef.value.validate()
    return true
  } catch {
    return false
  }
}

/** 重置校验 */
function resetFields(): void {
  formRef.value?.resetFields()
}

defineExpose({ validate, resetFields })
</script>

<style scoped>
.dynamic-form {
  max-width: 640px;
}
</style>
