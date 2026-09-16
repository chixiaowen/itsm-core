#!/usr/bin/env bash
# ============================================================
# itsm-core 日志查看脚本
# ============================================================
# 用法：
#   ./scripts/logs.sh              # 跟踪全部服务日志
#   ./scripts/logs.sh app          # 仅后端
#   ./scripts/logs.sh frontend     # 仅前端
#   ./scripts/logs.sh postgres     # 仅数据库
#   ./scripts/logs.sh app 200      # 后端最近 200 行（不跟踪）
# ============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_ROOT}"

SERVICE="${1:-}"
TAIL="${2:-}"

if [ -n "${SERVICE}" ] && [ -n "${TAIL}" ]; then
  docker compose logs --tail "${TAIL}" "${SERVICE}"
elif [ -n "${SERVICE}" ]; then
  docker compose logs -f "${SERVICE}"
else
  docker compose logs -f
fi
