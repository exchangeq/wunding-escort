#!/usr/bin/env bash
# 冒烟测试：起服务 → 前台接口 → 登录 → 上传 → 上架 → 下单 → 查单 → 改状态
# 用法：bash scripts/smoke.sh python  或  bash scripts/smoke.sh go
set -euo pipefail

BACKEND="${1:-python}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PORT="${SMOKE_PORT:-}"
if [ -z "$PORT" ]; then
  for candidate in $(seq 18080 18160); do
    if ! (exec 3<>"/dev/tcp/127.0.0.1/${candidate}") 2>/dev/null; then
      PORT="$candidate"
      break
    fi
  done
fi
PORT="${PORT:-18080}"

TMP="$(mktemp -d)"
BASE="http://127.0.0.1:${PORT}"

export PORT HOST=127.0.0.1
export DATA_DIR="$TMP/data"
export UPLOAD_DIR="$TMP/up"
export ADMIN_USER="smoke"
export ADMIN_PASS="smoke-123456"
export SESSION_SECRET="smoke-only-secret"

if [ "$BACKEND" = "go" ]; then
  BIN="${GO_BIN:-$TMP/wunding-go}"
  (cd server-go && go build -trimpath -o "$BIN" .)
  "$BIN" > "$TMP/server.log" 2>&1 &
else
  python3 server-py/app.py > "$TMP/server.log" 2>&1 &
fi
PID=$!

cleanup() {
  kill "$PID" 2>/dev/null || true
  wait "$PID" 2>/dev/null || true
  rm -rf "$TMP"
}
trap cleanup EXIT

pass() { echo "✓ $1"; }
die() {
  echo "✗ $1"
  echo "--- server.log ---"
  tail -n 40 "$TMP/server.log" 2>/dev/null || true
  exit 1
}

READY=""
for _ in $(seq 1 60); do
  CODE="$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/health" || true)"
  if [ "$CODE" = "200" ]; then READY=1; break; fi
  sleep 0.25
done
[ -n "$READY" ] || die "服务没起来"
pass "服务启动（$BACKEND 后端，端口 $PORT）"

get() { curl -fsS "$1" || die "GET $1 失败"; }
contains() { [[ "$1" == *"$2"* ]]; }

json_get() {
  python3 -c '
import json, sys
v = json.load(sys.stdin)
for key in sys.argv[1].split("."):
    v = v[int(key)] if isinstance(v, list) else v[key]
sys.stdout.write(str(v))
' "$1"
}

SITE="$(get "$BASE/api/site")"
contains "$SITE" "无定科技" || die "站点信息不对"
COUNT="$(printf '%s' "$SITE" | json_get "data.productCount")"
[ "$COUNT" -ge 8 ] || die "默认商品数量异常：$COUNT"
pass "站点和默认商品就绪（$COUNT 个）"

LIST="$(get "$BASE/api/products?game=%E7%8E%8B%E8%80%85%E8%8D%A3%E8%80%80")"
[ "$(printf '%s' "$LIST" | json_get "data.total")" -ge 1 ] || die "筛选结果为空"
pass "商品筛选可用"

PAGE="$(curl -fsS "$BASE/")"
contains "$PAGE" "无定科技" || die "首页没有版权信息"
contains "$PAGE" "后台入口" || die "首页导航缺失"
curl -fsS "$BASE/admin/" | grep -c "运营后台" > /dev/null || die "后台页面异常"
pass "前台与后台页面正常"

CODE="$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/admin/orders")"
[ "$CODE" = "401" ] || die "后台接口没鉴权（$CODE）"
pass "未登录访问后台返回 401"

TOKEN="$(curl -fsS -X POST "$BASE/api/admin/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}" | json_get "data.token")"
[ -n "$TOKEN" ] || die "登录失败"
AUTH="Authorization: Bearer $TOKEN"
pass "管理员登录"

PNG="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
COVER="$(curl -fsS -X POST "$BASE/api/admin/upload" -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{\"data\":\"$PNG\"}" | json_get "data.url")"
[ -f "$TMP/up/$(basename "$COVER")" ] || die "上传文件没有落盘"
pass "图片上传（$COVER）"

PROD_ID="$(curl -fsS -X POST "$BASE/api/admin/products" -H "$AUTH" -H 'Content-Type: application/json' -d "{
  \"title\":\"测试用商品 王者段位\",
  \"game\":\"王者荣耀\",
  \"category\":\"段位护航\",
  \"price\":99.5,
  \"unit\":\"段\",
  \"stock\":5,
  \"tags\":[\"测试\"],
  \"includes\":[\"进度反馈\"],
  \"desc\":\"冒烟测试临时商品\",
  \"cover\":\"$COVER\",
  \"status\":\"on\"
}" | json_get "data.id")"
[ -n "$PROD_ID" ] || die "上架失败"
contains "$(get "$BASE/api/products/$PROD_ID")" "测试用商品" || die "前台看不到新商品"
pass "后台上架的商品在前台可见（id=$PROD_ID）"

ORDER="$(curl -fsS -X POST "$BASE/api/orders" -H 'Content-Type: application/json' -d "{
  \"productId\":$PROD_ID,
  \"qty\":2,
  \"contactName\":\"测试\",
  \"contactPhone\":\"13800000000\",
  \"gameAccount\":\"SmokeTest\",
  \"server\":\"安卓微信区\",
  \"note\":\"冒烟订单\"
}")"
NO="$(printf '%s' "$ORDER" | json_get "data.orderNo")"
AMOUNT="$(printf '%s' "$ORDER" | json_get "data.amount")"
contains "$NO" "WD" || die "订单号异常：$NO"
AMOUNT_INT="$(python3 -c "print(round(float('$AMOUNT')))")"
[ "$AMOUNT_INT" = "199" ] || die "金额算错：$AMOUNT"
pass "下单成功（$NO，¥$AMOUNT）"

# ---- 支付：凭证 → 后台确认 ----
curl -fsS -X POST "$BASE/api/orders/$NO/proof" -H 'Content-Type: application/json' \
  -d "{\"data\":\"$PNG\"}" | grep -c '"review"' > /dev/null || die "付款凭证上传失败"
pass "付款凭证上传，状态转为待确认"

curl -fsS -X PUT "$BASE/api/admin/orders/$NO/payment" -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"action":"confirm"}' | grep -c '"paid"' > /dev/null || die "后台确认到账失败"
contains "$(get "$BASE/api/orders/$NO")" '"payStatus":"paid"' || die "查单看不到已到账"
pass "后台确认到账，查单同步"

# ---- 支付：易支付下单 + 异步通知验签 ----
curl -fsS -X PUT "$BASE/api/admin/settings" -H "$AUTH" -H 'Content-Type: application/json' -d '{
  "payment":{
    "manual":{"enabled":true,"wechatQr":"","alipayQr":"","note":"冒烟备注"},
    "epay":{"enabled":true,"gateway":"https://pay.smoke.local","pid":"1001","key":"smoke-epay-key","type":"alipay","name":"在线支付"}
  }
}' > /dev/null || die "支付设置保存失败"

python3 - "$BASE" "$AUTH" <<'PY'
import hashlib, json, sys, urllib.parse, urllib.request
base, auth = sys.argv[1], sys.argv[2]

def req(path, method='GET', body=None, token=None, raw=False):
    data = json.dumps(body).encode() if body is not None else None
    headers = {'Content-Type': 'application/json'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    with urllib.request.urlopen(urllib.request.Request(base + path, method=method, data=data, headers=headers)) as r:
        text = r.read().decode()
        return text if raw else json.loads(text)

order = req('/api/orders', 'POST', {
    'productId': 1, 'qty': 1, 'contactName': '冒烟支付',
    'contactPhone': '13900000000', 'gameAccount': 'SmokePay'
})['data']
no = order['orderNo']
url = req('/api/orders/%s/pay' % no, 'POST', {})['data']['payUrl']
assert 'sign=' in url, '没生成签名'
values = {k: v[0] for k, v in urllib.parse.parse_qs(urllib.parse.urlparse(url).query).items()
          if k not in ('sign', 'sign_type')}
values.update({'trade_no': 'SMOKE20261121', 'trade_status': 'TRADE_SUCCESS'})
values['sign'] = hashlib.md5(
    ('&'.join('%s=%s' % (k, values[k]) for k in sorted(values)) + 'smoke-epay-key').encode()
).hexdigest()
values['sign_type'] = 'MD5'
resp = req('/api/pay/notify?' + urllib.parse.urlencode(values), raw=True)
assert resp.strip() == 'success', '通知返回：' + resp
track = req('/api/orders/' + no)['data']
assert track['payStatus'] == 'paid' and track['payMethod'] == 'epay', track
values['sign'] = '0' * 32
bad = req('/api/pay/notify?' + urllib.parse.urlencode(values), raw=True)
assert bad.strip() == 'fail', '错误签名没被拒绝'
print('✓ 在线支付下单链接与异步通知验签正常')
print('✓ 错误签名被拒绝')
PY

TRACK="$(get "$BASE/api/orders/$NO")"
contains "$TRACK" "138****0000" || die "手机号没脱敏"
pass "订单查询正常且手机号脱敏"

curl -fsS -X PUT "$BASE/api/admin/orders/$NO" -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"status":"done"}' | grep -c '"done"' > /dev/null || die "改状态失败"
curl -fsS "$BASE/api/admin/overview" -H "$AUTH" | grep -c '"revenue"' > /dev/null || die "概览接口异常"
pass "订单状态流转与概览正常"

curl -fsS -X PUT "$BASE/api/admin/settings" -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"notice":"冒烟公告","copyright":"© 2026 无定科技 版权所有"}' > /dev/null || die "改设置失败"
contains "$(get "$BASE/api/site")" "冒烟公告" || die "设置没生效"
pass "站点设置能改并同步前台"

echo
echo "冒烟测试通过（$BACKEND）"
