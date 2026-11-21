#!/usr/bin/env bash
# Linux / macOS 一键启动（Python 后端）
set -euo pipefail
cd "$(dirname "$0")"

export PORT="${PORT:-8080}"
export HOST="${HOST:-0.0.0.0}"

exec python3 server-py/app.py
