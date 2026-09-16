# ============================================================
# itsm-core 前端多阶段构建 Dockerfile
# ============================================================
# 阶段 1：Node 构建 SPA 产物（vue-tsc 类型检查 + vite build）
# 阶段 2：nginx 托管静态产物，并把 /api 反代到后端服务
#
# 构建：docker build -t itsm-core-frontend:latest ./frontend
# 说明：容器内前端以**相对路径**请求 API（VITE_API_BASE=/api/v1），
#       由 nginx 转发到 compose 网络内的 `app:8080`，因此不存在跨域问题。
#
# 环境约束（与后端一致）：
#   - 默认 npm 官方源在中国网络下可能较慢，默认走 npmmirror，可用 --build-arg 覆盖

# ---------- 阶段 1：构建 ----------
FROM node:22-alpine AS builder

ARG NPM_REGISTRY=https://registry.npmmirror.com
ARG VITE_API_BASE=/api/v1
ARG VITE_APP_TITLE=ITSM-CORE

ENV VITE_API_BASE=${VITE_API_BASE} \
    VITE_APP_TITLE=${VITE_APP_TITLE}

WORKDIR /app

# 先拷贝依赖清单，利用 Docker 层缓存
COPY package.json package-lock.json* ./

# 有 lock 文件用 npm ci 保证可复现，否则退化为 npm install
RUN if [ -f package-lock.json ]; then \
        npm ci --no-audit --no-fund --registry=${NPM_REGISTRY}; \
    else \
        npm install --no-audit --no-fund --registry=${NPM_REGISTRY}; \
    fi

# 拷贝源码并构建（build 脚本内含 vue-tsc --noEmit 类型检查）
COPY . .
RUN npm run build

# ---------- 阶段 2：运行 ----------
FROM nginx:1.27-alpine

# 时区数据（日志时间可读）
RUN apk add --no-cache tzdata

# 用自定义配置替换默认站点配置
RUN rm -f /etc/nginx/conf.d/default.conf
COPY nginx.conf /etc/nginx/conf.d/itsm-core.conf

# 拷贝构建产物
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80

# 健康检查：nginx 内建 /healthz
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
    CMD wget -q -O /dev/null http://127.0.0.1/healthz || exit 1

CMD ["nginx", "-g", "daemon off;"]
