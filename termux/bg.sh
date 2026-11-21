#!/data/data/com.termux/files/usr/bin/bash
# 放到后台跑，关掉终端也不停（手机别杀 Termux 进程）
set -e
cd "$(dirname "$0")/.."

termux-wake-lock 2>/dev/null || true

export PORT="${PORT:-8080}"
export HOST="${HOST:-0.0.0.0}"
mkdir -p data

if [ -f data/termux.pid ] && kill -0 "$(cat data/termux.pid)" 2>/dev/null; then
  echo "已经在跑了，PID $(cat data/termux.pid)"
  exit 0
fi

nohup python3 server-py/app.py >> data/termux.log 2>&1 &
echo $! > data/termux.pid
sleep 1

echo "已启动，PID $(cat data/termux.pid)"
echo "地址 http://127.0.0.1:${PORT}/，日志 data/termux.log"
