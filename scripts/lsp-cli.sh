#!/usr/bin/env sh
set -eu

# lsp-cli.sh 在 release archive 内按当前平台选择二进制，并默认连接线上 gate。
DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  arm64|aarch64) ARCH=arm64 ;;
  x86_64|amd64) ARCH=amd64 ;;
  *) echo "不支持的 CPU 架构: $ARCH" >&2; exit 1 ;;
esac

BIN="$DIR/lsp-cli_${OS}_${ARCH}"
if [ ! -x "$BIN" ]; then
  BIN="$DIR/../lsp-cli_${OS}_${ARCH}"
fi
if [ ! -x "$BIN" ]; then
  BIN="$DIR/lsp-cli"
fi
if [ ! -x "$BIN" ]; then
  echo "未找到可执行的 lsp-cli 二进制" >&2
  exit 1
fi

exec "$BIN" --ws "${LSP_CLI_WS:-wss://racoo.cn/ws}" "$@"
