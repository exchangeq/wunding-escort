#!/usr/bin/env bash
# 独立服务器上跑 Go 版：有二进制就直接跑，没有就用本机 go 编译一份
set -euo pipefail
cd "$(dirname "$0")"

export PORT="${PORT:-8080}"
export HOST="${HOST:-0.0.0.0}"
export DATA_DIR="${DATA_DIR:-data}"

BIN="./wunding-escort"
if [ ! -x "$BIN" ]; then
  echo "没找到二进制，先用本机 Go 编译…"
  (cd server-go && go build -trimpath -ldflags "-s -w" -o "../wunding-escort" .)
fi

exec "$BIN"
