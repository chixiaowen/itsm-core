<template>
  <el-select
    :model-value="innerValue"
    :multiple="multiple"
    :filterable="filterable"
    :clearable="clearable"
    :disabled="disabled"
    :placeholder="placeholder"
    :loading="loading"
    :collapse-tags="multiple"
    collapse-tags-tooltip
    @update:model-value="onUpdate"
  >
    <el-option
      v-for="opt in options"
      :key="opt.id"
      :label="optionLabel(opt)"
      :value="opt.id"
    />
  </el-select>
</template>

<script setup lang="ts" generic="T = number | number[] | null | undefined">
/**
 * 用户下拉选择组件（替换各处「输入用户ID」）。
 *
 * - 数据源：GET /users/options（支持 ?role= 过滤），任意已登录用户可访问
 * - role 支持逗号分隔多角色（如 "agent,resolver"），内部并行拉取后按 id 去重合并
 * - 模块级缓存：以 role 组合为键缓存结果，避免同一 role 反复请求
 * - 选项显示：`display_name（角色中文名）`
 * - 泛型 T 使 v-model 可适配 `number` / `number | ''` / `number[]` 等模型
 */
import { computed, onMounted, ref, watch } from 'vue'
import { ROLE_LABELS, type Role } from '@/types/common'
import type { UserOption } from '@/types/platform'
import { listUserOptions } from '@/api/platform'

const props = withDefaults(
  defineProps<{
    modelValue?: T
    /** 角色过滤，支持逗号分隔多角色；为空表示不过滤 */
    role?: string
    multiple?: boolean
    placeholder?: string
    clearable?: boolean
    disabled?: boolean
    filterable?: boolean
  }>(),
  {
    role: '',
    multiple: false,
    placeholder: '请选择用户',
    clearable: true,
    disabled: false,
    filterable: true
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: T): void
  (e: 'change', value: T): void
}>()

/** 模块级缓存：role 组合 -> 选项列表 */
const optionsCache = new Map<string, UserOption[]>()

const options = ref<UserOption[]>([])
const loading = ref(false)

const innerValue = computed(() => {
  if (props.multiple) return Array.isArray(props.modelValue) ? props.modelValue : []
  return (props.modelValue as number | null | undefined) ?? undefined
})

function optionLabel(opt: UserOption): string {
  const roleName = ROLE_LABELS[opt.role as Role] ?? opt.role
  return `${opt.display_name}（${roleName}）`
}

/** 拉取选项：单角色单次请求，多角色并行合并去重 */
async function fetchOptions(roleKey: string): Promise<UserOption[]> {
  const roles = roleKey
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
  if (roles.length <= 1) {
    return listUserOptions(roles[0] ? { role: roles[0] } : undefined)
  }
  const groups = await Promise.all(roles.map((role) => listUserOptions({ role })))
  const merged = new Map<number, UserOption>()
  groups.flat().forEach((opt) => merged.set(opt.id, opt))
  return [...merged.values()]
}

async function load(roleKey: string): Promise<void> {
  const key = roleKey.trim()
  if (optionsCache.has(key)) {
    options.value = optionsCache.get(key) ?? []
    return
  }
  loading.value = true
  try {
    const list = await fetchOptions(key)
    optionsCache.set(key, list)
    options.value = list
  } catch {
    // 拉取失败：置空并由拦截器提示，组件保持可用
    options.value = []
  } finally {
    loading.value = false
  }
}

function onUpdate(value: T): void {
  emit('update:modelValue', value)
  emit('change', value)
}

watch(
  () => props.role,
  (role) => {
    void load(role ?? '')
  }
)

onMounted(() => {
  void load(props.role ?? '')
})
</script>
