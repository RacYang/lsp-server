#!/usr/bin/env bash
set -euo pipefail

# drills-dev.sh 启动一组本地联调进程，把后端、客户端与帧 dump 三段日志按时间戳归档到
# tmp/drills/<ts>/ 下，配合 docs/spec/player-journey.md 与 docs/spec/architecture-gaps.md
# 对照玩家旅程 spec 条款 → 实际帧 → 后端日志切片。
#
# 用法：
#   scripts/drills-dev.sh start           # 起后端 cmd/all（默认 configs/dev.yaml）
#   scripts/drills-dev.sh start --cli     # 同时拉一个本地 cli，开启 LSP_FRAME_LOG
#   scripts/drills-dev.sh stop            # 停掉本脚本拉起的进程
#   scripts/drills-dev.sh dir             # 打印当前 drill 目录
#
# 不替代 make verify-*；它只是开发期看帧的便利脚本。

REPO_ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "${REPO_ROOT}"

DRILL_LATEST="tmp/drills/latest"
TS=$(date +"%Y%m%d-%H%M%S")
DRILL_DIR="tmp/drills/${TS}"

cmd=${1:-start}
shift || true

start() {
  local with_cli=0
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --cli) with_cli=1 ;;
      *) echo "未知参数: $1" >&2; exit 2 ;;
    esac
    shift
  done

  mkdir -p "${DRILL_DIR}"
  ln -sfn "${TS}" "tmp/drills/latest"

  if [[ -z "${LSP_CONFIG:-}" ]]; then
    export LSP_CONFIG="configs/dev.yaml"
  fi
  export LOGX_FORMAT="${LOGX_FORMAT:-json}"

  echo "[drill] drill 目录: ${DRILL_DIR}"
  echo "[drill] 后端配置: ${LSP_CONFIG}"

  (
    cd "${REPO_ROOT}"
    nohup go run ./cmd/all \
      > "${DRILL_DIR}/backend.log" 2>&1 &
    echo $! > "${DRILL_DIR}/backend.pid"
  )
  echo "[drill] 后端 PID=$(cat "${DRILL_DIR}/backend.pid") 日志=${DRILL_DIR}/backend.log"

  if [[ ${with_cli} -eq 1 ]]; then
    export LSP_FRAME_LOG="${REPO_ROOT}/${DRILL_DIR}/frames.jsonl"
    echo "[drill] 客户端帧 dump: ${LSP_FRAME_LOG}"
    echo "[drill] 启动 cli（前台）按 Ctrl-C 退出；后端继续在后台。"
    exec go run ./cmd/cli --ws "ws://127.0.0.1:8080/ws"
  fi

  echo "[drill] 后端已启动。手动拉 cli 时记得："
  echo "        export LSP_FRAME_LOG=\"${REPO_ROOT}/${DRILL_DIR}/frames.jsonl\""
  echo "        go run ./cmd/cli --ws ws://127.0.0.1:8080/ws"
}

stop() {
  if [[ ! -L "${DRILL_LATEST}" ]]; then
    echo "[drill] 未找到 ${DRILL_LATEST}，无需停止"
    return 0
  fi
  target=$(readlink "${DRILL_LATEST}")
  pid_file="tmp/drills/${target}/backend.pid"
  if [[ -f "${pid_file}" ]]; then
    pid=$(cat "${pid_file}")
    if kill -0 "${pid}" 2>/dev/null; then
      echo "[drill] 停止后端 PID=${pid}"
      kill "${pid}" || true
    fi
    rm -f "${pid_file}"
  fi
}

dir() {
  if [[ -L "${DRILL_LATEST}" ]]; then
    target=$(readlink "${DRILL_LATEST}")
    echo "tmp/drills/${target}"
  else
    echo "未启动任何 drill" >&2
    exit 1
  fi
}

case "${cmd}" in
  start) start "$@" ;;
  stop)  stop ;;
  dir)   dir ;;
  *) echo "用法: $0 {start|stop|dir} [--cli]" >&2; exit 2 ;;
esac
