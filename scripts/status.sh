#!/usr/bin/env bash
# ============================================================
# itsm-core 状态查看脚本
# ============================================================
# 输出：容器状态、健康检查、端口映射、接口连通性探测
# 用法：./scripts/status.sh
# ============================================================
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_ROOT}"

FRONTEND_PORT="${FRONTEND_PORT:-8081}"
API_PORT="${API_PORT:-8080}"

log() { printf '\033[1;34m[itsm-core]\033[0m %s\n' "$*"; }

# ---- Docker 可用性 ----
if ! docker info >/dev/null 2>&1; then
  log "Docker 守护进程不可用。"
  command -v colima >/dev/null 2>&1 && colima status 2>&1 | sed 's/^/  /'
  exit 1
fi

log "容器状态"
docker compose ps 2>/dev/null

echo
log "健康检查"
for c in itsm-core-postgres itsm-core-app itsm-core-frontend; do
  status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${c}" 2>/dev/null || echo "not-created")"
  printf '  %-24s %s\n' "${c}" "${status}"
done

echo
log "接口探测"
# 本机若有 http_proxy，curl 访问 localhost 会走代理而失败，故统一 --noproxy
probe() {
  local url="$1" label="$2"
  local code
  code="$(curl -s --noproxy '*' -o /dev/null -w '%{http_code}' --max-time 5 "${url}" 2>/dev/null || echo "000")"
  if [ "${code}" = "000" ]; then
    printf '  %-22s %s\n' "${label}" "不可达（服务未启动？）"
  else
    printf '  %-22s HTTP %s\n' "${label}" "${code}"
  fi
}
probe "http://localhost:${API_PORT}/healthz" "后端 /healthz"
probe "http://localhost:${FRONTEND_PORT}/healthz" "前端 /healthz"
probe "http://localhost:${API_PORT}/api/v1/roles" "后端鉴权探针（期望 401）"

echo
log "端口映射"
docker compose ps --format '{{.Service}}: {{.Ports}}' 2>/dev/null
