#!/data/data/com.termux/files/usr/bin/bash
# 停掉后台进程
cd "$(dirname "$0")/.." || exit 0

if [ -f data/termux.pid ]; then
  PID="$(cat data/termux.pid)"
  kill "$PID" 2>/dev/null && echo "已停止 PID ${PID}" || echo "进程已经不在了"
  rm -f data/termux.pid
else
  echo "没有找到 pid 文件，试着手动 pkill -f server-py/app.py"
fi

termux-wake-unlock 2>/dev/null || true
