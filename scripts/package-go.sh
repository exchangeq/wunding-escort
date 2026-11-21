#!/usr/bin/env bash
# 交叉编译 Go 版并按平台打包（自带 web 和 seed.json，解压即跑）
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="$(tr -d '[:space:]' < VERSION)"
mkdir -p dist

build_target() {
  local goos="$1" goarch="$2" suffix="$3"
  local name="wunding-escort-go-${VERSION}-${goos}-${goarch}"
  local stage="$ROOT/dist/.stage/$name"

  rm -rf "$stage"
  mkdir -p "$stage"

  echo "编译 ${goos}/${goarch} …"
  (cd server-go && GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w" -o "$stage/wunding-escort${suffix}" .)

  cp -R web "$stage/"
  cp seed.json README.md LICENSE VERSION "$stage/"

  cat > "$stage/run.sh" <<EOF
#!/usr/bin/env bash
cd "\$(dirname "\$0")"
export PORT="\${PORT:-8080}"
export HOST="\${HOST:-0.0.0.0}"
export DATA_DIR="\${DATA_DIR:-data}"
mkdir -p "\$DATA_DIR"
exec ./wunding-escort${suffix}
EOF
  chmod +x "$stage/run.sh" "$stage/wunding-escort${suffix}"

  rm -f "$ROOT/dist/$name.zip"
  (cd "$ROOT/dist/.stage" && zip -rq "$ROOT/dist/$name.zip" "$name" -x "*.DS_Store")
  rm -rf "$stage"
  echo "打包完成：dist/$name.zip"
}

build_target linux amd64 ""
build_target linux arm64 ""

rm -rf "$ROOT/dist/.stage"
ls -lh "$ROOT/dist/"*.zip
