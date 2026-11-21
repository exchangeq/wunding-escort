#!/usr/bin/env bash
# 打包 Python 版（Termux / 普通服务器都能用）
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="$(tr -d '[:space:]' < VERSION)"
NAME="wunding-escort-python-${VERSION}"
STAGE="$ROOT/dist/.stage/$NAME"

rm -rf "$STAGE"
mkdir -p "$STAGE/server-py" "$STAGE/data"

cp -R web termux server-py deploy "$STAGE/"
cp seed.json README.md LICENSE VERSION start.sh start-go.sh "$STAGE/" 2>/dev/null || true
rm -f "$STAGE/start-go.sh"

find "$STAGE" -name '__pycache__' -type d -prune -exec rm -rf {} + 2>/dev/null || true

cat > "$STAGE/run.sh" <<'EOF'
#!/usr/bin/env bash
cd "$(dirname "$0")"
export PORT="${PORT:-8080}"
export HOST="${HOST:-0.0.0.0}"
exec python3 server-py/app.py
EOF
chmod +x "$STAGE/run.sh" "$STAGE/start.sh" "$STAGE/termux/"*.sh

mkdir -p "$ROOT/dist"
rm -f "$ROOT/dist/$NAME.zip"
(cd "$ROOT/dist/.stage" && zip -rq "$ROOT/dist/$NAME.zip" "$NAME" -x "*.DS_Store")
rm -rf "$ROOT/dist/.stage"
echo "打包完成：dist/$NAME.zip"
