<template>
  <div class="error-page">
    <el-result :icon="icon" :title="title" :sub-title="subTitle">
      <template #extra>
        <el-button type="primary" @click="router.push('/dashboard')">返回工作台</el-button>
        <el-button @click="router.back()">返回上一页</el-button>
      </template>
    </el-result>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'

const props = withDefaults(defineProps<{ code?: number }>(), { code: 404 })

const router = useRouter()

const icon = computed(() => (props.code === 403 ? 'warning' : 'error'))
const title = computed(() => (props.code === 403 ? '403 无权限访问' : '404 页面不存在'))
const subTitle = computed(() =>
  props.code === 403 ? '当前角色无权访问该页面，请联系管理员。' : '请检查地址是否正确，或返回工作台继续操作。'
)
</script>

<style scoped>
.error-page {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding-top: 80px;
}
</style>
