<template>
  <div class="login-page">
    <el-card class="login-card" shadow="always">
      <div class="login-title">ITSM-CORE 管理台</div>
      <div class="login-subtitle">IT 服务管理平台</div>
      <el-form ref="formRef" :model="form" :rules="rules" @keyup.enter="submit">
        <el-form-item prop="username">
          <el-input v-model="form.username" placeholder="用户名" size="large" clearable />
        </el-form-item>
        <el-form-item prop="password">
          <el-input
            v-model="form.password"
            type="password"
            placeholder="密码"
            size="large"
            show-password
          />
        </el-form-item>
        <el-button type="primary" size="large" :loading="loading" class="login-btn" @click="submit">
          登录
        </el-button>
      </el-form>
      <div class="login-tip">默认管理员账号：admin / admin123</div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/stores/user'
import type { LoginReq } from '@/types/platform'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive<LoginReq>({ username: 'admin', password: '' })

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }]
}

async function submit(): Promise<void> {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  loading.value = true
  try {
    const user = await userStore.login({ ...form })
    ElMessage.success(`欢迎回来，${user.display_name}`)
    const redirect = (route.query.redirect as string) || '/'
    await router.push(redirect)
  } catch {
    // 错误提示已在 http 拦截器统一处理
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #1f2d3d 0%, #2b6cb0 100%);
}
.login-card {
  width: 380px;
  padding: 8px 12px;
}
.login-title {
  font-size: 22px;
  font-weight: 700;
  text-align: center;
  color: #2b6cb0;
}
.login-subtitle {
  text-align: center;
  color: #909399;
  margin: 4px 0 20px;
  font-size: 13px;
}
.login-btn {
  width: 100%;
}
.login-tip {
  margin-top: 14px;
  text-align: center;
  color: #c0c4cc;
  font-size: 12px;
}
</style>
