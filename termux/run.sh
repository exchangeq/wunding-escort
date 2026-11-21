#!/data/data/com.termux/files/usr/bin/bash
# 前台运行，Ctrl+C 停止
set -e
cd "$(dirname "$0")/.."

termux-wake-lock 2>/dev/null || true

export PORT="${PORT:-8080}"
export HOST="${HOST:-0.0.0.0}"
mkdir -p data

echo "端口 ${PORT}，手机浏览器打开 http://127.0.0.1:${PORT}/"
echo "同一 WiFi 下用电脑访问：http://<手机IP>:${PORT}/"
echo

exec python3 server-py/app.py
