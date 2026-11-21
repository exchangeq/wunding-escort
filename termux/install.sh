#!/data/data/com.termux/files/usr/bin/bash
# Termux 首次准备：装 Python，不需要 pip 装任何包
set -e

echo "== 更新软件源 =="
pkg update -y
pkg install -y python

echo "== 版本 =="
python --version

echo
echo "装好了。下一步："
echo "  cd $(pwd)/.."
echo "  bash termux/run.sh"
