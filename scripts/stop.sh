#!/usr/bin/env bash
# ============================================================
# itsm-core 停止脚本
# ============================================================
# 用法：
#   ./scripts/stop.sh          # 停止并移除容器（保留数据库数据卷）
#   ./scripts/stop.sh --purge  # 同时删除数据卷（清空数据库，谨慎）
#   ./scripts/stop.sh --vm     # 停止后一并关闭 colima 虚拟机
# ============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_ROOT}"

PURGE=0
STOP_VM=0
for arg in "$@"; do
  case "${arg}" in
    --purge) PURGE=1 ;;
    --vm)    STOP_VM=1 ;;
    -h|--help) sed -n '2,10p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "未知参数：${arg}" >&2; exit 2 ;;
  esac
done

log() { printf '\033[1;34m[itsm-core]\033[0m %s\n' "$*"; }

if [ "${PURGE}" -eq 1 ]; then
  log "停止服务并删除数据卷（数据库将被清空）..."
  docker compose down -v
else
  log "停止服务（保留数据卷）..."
  docker compose down
fi

if [ "${STOP_VM}" -eq 1 ]; then
  if command -v colima >/dev/null 2>&1; then
    log "停止 colima 虚拟机..."
    colima stop
  fi
fi

log "已停止。"
