#!/usr/bin/env bash
# ============================================================
# itsm-core 停止脚本（幂等：重复执行或服务未运行时也返回成功）
# ============================================================
# 用法：
#   ./scripts/stop.sh          # 停止并移除容器（保留数据库数据卷）
#   ./scripts/stop.sh --purge  # 同时删除数据卷（清空数据库，谨慎）
#   ./scripts/stop.sh --vm     # 停止后一并关闭 colima 虚拟机
# ============================================================
# 说明：不使用 set -e。停止是幂等操作，任何一步失败都不应
#       中断后续步骤（例如 Docker 已停时仍应继续执行 --vm）。
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_ROOT}" || exit 1

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

log()  { printf '\033[1;34m[itsm-core]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[警告]\033[0m %s\n' "$*" >&2; }

# ---- 1. 停止容器（Docker 不可用时跳过，不视为错误）----
if docker info >/dev/null 2>&1; then
  if [ "${PURGE}" -eq 1 ]; then
    log "停止服务并删除数据卷（数据库将被清空）..."
    docker compose down -v || warn "docker compose down -v 执行异常，请手动检查：docker compose ps"
  else
    log "停止服务（保留数据卷）..."
    docker compose down || warn "docker compose down 执行异常，请手动检查：docker compose ps"
  fi
else
  log "Docker 未运行，无容器需要停止。"
fi

# ---- 2. 按需关闭 colima 虚拟机 ----
if [ "${STOP_VM}" -eq 1 ]; then
  if command -v colima >/dev/null 2>&1; then
    if colima status >/dev/null 2>&1; then
      log "停止 colima 虚拟机..."
      colima stop || warn "colima stop 执行异常，请手动检查：colima status"
    else
      log "colima 已处于停止状态。"
    fi
  else
    warn "未安装 colima，跳过。"
  fi
fi

log "已停止。"
exit 0
