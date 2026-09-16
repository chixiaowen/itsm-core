import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import router from './router'
import './index.css'

const app = createApp(App)

// Pinia 必须先于 Router 安装：路由守卫中会访问 store。
app.use(createPinia())
app.use(router)
app.use(ElementPlus)

app.mount('#app')
