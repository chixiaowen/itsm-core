#!/usr/bin/env bash
# ============================================================
# itsm-core 一键启动脚本（Docker Compose）
# ============================================================
# 启动顺序：PostgreSQL → 后端 API → 前端管理台
# 依赖条件：各服务均配置 healthcheck，按 healthy 逐级推进
#
# 用法：
#   ./scripts/start.sh              # 构建并启动全部（默认）
#   ./scripts/start.sh --db-only    # 仅启动 PostgreSQL（配合本地 go run 开发）
#   ./scripts/start.sh --no-build   # 不重新构建镜像，直接启动
#   ./scripts/start.sh --rebuild    # 忽略缓存强制重建镜像
#   ./scripts/start.sh --api-only   # 启动 PostgreSQL + 后端
# ============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_ROOT}"

# ---- 端口（与 docker-compose.yml 的默认值保持一致）----
FRONTEND_PORT="${FRONTEND_PORT:-8081}"
API_PORT="${API_PORT:-8080}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"

# ---- 参数解析 ----
DB_ONLY=0
API_ONLY=0
DO_BUILD=1
REBUILD=0
for arg in "$@"; do
  case "${arg}" in
    --db-only)  DB_ONLY=1 ;;
    --api-only) API_ONLY=1 ;;
    --no-build) DO_BUILD=0 ;;
    --rebuild)  REBUILD=1 ;;
    -h|--help)
      sed -n '2,14p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *) echo "未知参数：${arg}（用 --help 查看用法）" >&2; exit 2 ;;
  esac
done

log()  { printf '\033[1;34m[itsm-core]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[警告]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[错误]\033[0m %s\n' "$*" >&2; exit 1; }

# ---- 1. 检查 Docker 可用性，必要时拉起 colima ----
if ! docker info >/dev/null 2>&1; then
  warn "Docker 守护进程不可用。"
  if command -v colima >/dev/null 2>&1; then
    log "检测到 colima，正在启动虚拟机（首次约需 30-60 秒）..."
    colima start
  else
    die "未安装 colima 且 Docker 不可用。请先安装 Docker 或 colima。"
  fi
fi
docker info >/dev/null 2>&1 || die "Docker 仍不可用，请检查 colima 状态（colima status）。"
log "Docker 就绪：$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo 'unknown')"

# ---- 2. 组装服务列表 ----
if [ "${DB_ONLY}" -eq 1 ]; then
  SERVICES="postgres"
elif [ "${API_ONLY}" -eq 1 ]; then
  SERVICES="postgres app"
else
  SERVICES="postgres app frontend"
fi

# ---- 3. 构建并启动 ----
if [ "${DO_BUILD}" -eq 1 ]; then
  if [ "${REBUILD}" -eq 1 ]; then
    log "强制重建镜像（--no-cache）..."
    docker compose build --no-cache ${SERVICES}
  else
    log "构建镜像（已存在且无变更时会走缓存）..."
    docker compose build ${SERVICES}
  fi
fi

log "启动服务：${SERVICES}"
docker compose up -d ${SERVICES}

# ---- 4. 等待各服务健康 ----
# 注：本机若设置 http_proxy，curl 访问 localhost 会走代理而失败，故统一加 --noproxy
wait_healthy() {
  local container="$1" name="$2" max="${3:-60}"
  local i=1
  while [ "${i}" -le "${max}" ]; do
    local status
    status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${container}" 2>/dev/null || echo "missing")"
    case "${status}" in
      healthy|running) log "${name} 就绪（${status}）"; return 0 ;;
      unhealthy)       die "${name} 健康检查失败，请查看日志：docker compose logs ${name}" ;;
      missing)         die "未找到容器 ${container}，请查看：docker compose ps" ;;
    esac
    sleep 2
    i=$((i + 1))
  done
  warn "${name} 在 $((max * 2)) 秒内未就绪，请手动查看：docker compose logs ${name}"
  return 1
}

echo
if printf '%s' "${SERVICES}" | grep -q postgres; then
  wait_healthy "itsm-core-postgres" "PostgreSQL"
fi
if printf '%s' "${SERVICES}" | grep -q app; then
  wait_healthy "itsm-core-app" "后端 API"
fi
if printf '%s' "${SERVICES}" | grep -q frontend; then
  wait_healthy "itsm-core-frontend" "前端管理台"
fi

# ---- 5. 输出访问信息 ----
# 说明：中文为双宽字符，用 printf 对齐会错位，故改用逐行列表
echo
log "启动完成。访问信息："
echo
if printf '%s' "${SERVICES}" | grep -q frontend; then
  echo "    • 前端管理台    http://localhost:${FRONTEND_PORT}"
fi
if printf '%s' "${SERVICES}" | grep -q app; then
  echo "    • 后端 API      http://localhost:${API_PORT}/api/v1"
  echo "    • 健康检查      http://localhost:${API_PORT}/healthz"
fi
echo "    • 数据库        localhost:${POSTGRES_PORT}（用户 itsm / 密码 itsm / 库 itsm_core）"

if printf '%s' "${SERVICES}" | grep -q frontend; then
  cat <<EOF

  演示账号（首次启动自动写入，密码统一 admin123）：
    admin / requestor01 / agent01 / resolver01
    pm01 / cm01 / cm02 / cmdb01

  常用命令：
    docker compose logs -f          # 跟踪全部日志
    ./scripts/status.sh             # 查看服务状态
    ./scripts/stop.sh               # 停止服务
EOF
fi
echo
