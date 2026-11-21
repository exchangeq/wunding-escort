"""接口实现：前台 / 后台共用一份逻辑，Python 与 Go 版返回结构完全一致。"""

import base64
import binascii
import json
import hashlib
import hmac
from datetime import date, datetime, timedelta
from pathlib import Path
from urllib.parse import urlencode

import security
import store
from util import (
    HttpError,
    flag,
    integer,
    mask_phone,
    now_iso,
    number,
    ok,
    order_no,
    string_list,
    text,
    uid,
)

ORDER_STATUS = ("pending", "processing", "done", "cancelled")
LOGIN_MAX = 6
LOGIN_WINDOW = 600
_login_attempts = {}


def _public_product(p):
    keys = (
        "id", "game", "category", "title", "subtitle", "cover", "price",
        "oldPrice", "unit", "tags", "desc", "includes", "stock", "sales",
        "status", "sort", "createdAt", "updatedAt",
    )
    return {k: p.get(k) for k in keys}


def _visible(db):
    return [p for p in db["products"] if p.get("status") != "off"]


def health(ctx):
    ok(ctx.h, {"status": "ok", "time": now_iso()})


def _payment_public(settings):
    pay = settings.get("payment") or {}
    manual = pay.get("manual") or {}
    epay = pay.get("epay") or {}
    epay_on = bool(epay.get("enabled")) and bool(epay.get("gateway")) and bool(epay.get("pid")) and bool(epay.get("key"))
    return {
        "manual": {
            "enabled": bool(manual.get("enabled", True)),
            "wechatQr": manual.get("wechatQr") or "",
            "alipayQr": manual.get("alipayQr") or "",
            "note": manual.get("note") or "",
        },
        "epay": {
            "enabled": epay_on,
            "name": epay.get("name") or "在线支付",
            "type": epay.get("type") or "alipay",
        },
    }


def site(ctx):
    db = store.load()
    items = _visible(db)
    settings = dict(db["settings"])
    settings["payment"] = _payment_public(db["settings"])
    games = {}
    for p in items:
        games[p.get("game") or "其他"] = games.get(p.get("game") or "其他", 0) + 1
    categories = sorted({p.get("category") for p in items if p.get("category")})
    ok(ctx.h, {
        "settings": settings,
        "games": [{"name": k, "count": v} for k, v in games.items()],
        "categories": categories,
        "productCount": len(items),
        "updatedAt": db["meta"].get("updatedAt") or db["meta"].get("createdAt"),
    })


def products(ctx):
    db = store.load()
    q = (ctx.query.get("q") or "").strip().lower()[:40]
    game = (ctx.query.get("game") or "").strip()[:40]
    category = (ctx.query.get("category") or "").strip()[:40]
    sort = (ctx.query.get("sort") or "default").strip()

    items = _visible(db)
    if game and game != "全部":
        items = [p for p in items if p.get("game") == game]
    if category and category != "全部":
        items = [p for p in items if p.get("category") == category]
    if q:
        def haystack(p):
            parts = [p.get("title") or "", p.get("subtitle") or "", p.get("game") or "",
                     p.get("category") or "", " ".join(p.get("tags") or [])]
            return " ".join(parts).lower()
        items = [p for p in items if q in haystack(p)]

    if sort == "sales":
        items.sort(key=lambda p: -(p.get("sales") or 0))
    elif sort == "priceAsc":
        items.sort(key=lambda p: p.get("price") or 0)
    elif sort == "priceDesc":
        items.sort(key=lambda p: -(p.get("price") or 0))
    elif sort == "new":
        items.sort(key=lambda p: str(p.get("createdAt")), reverse=True)
    else:
        items.sort(key=lambda p: (p.get("sort") or 0, p.get("id") or 0))

    ok(ctx.h, {"total": len(items), "items": [_public_product(p) for p in items]})


def product_detail(ctx):
    db = store.load()
    try:
        pid = int(ctx.params["id"])
    except (KeyError, ValueError):
        raise HttpError(404, "商品不存在或已下架", "PRODUCT_NOT_FOUND")
    for p in db["products"]:
        if int(p.get("id") or 0) == pid and p.get("status") != "off":
            return ok(ctx.h, _public_product(p))
    raise HttpError(404, "商品不存在或已下架", "PRODUCT_NOT_FOUND")


def create_order(ctx):
    body = ctx.body
    try:
        pid = int(body.get("productId"))
    except (TypeError, ValueError):
        raise HttpError(400, "请选择要购买的服务", "VALIDATION")
    db = store.load()
    product = next((p for p in db["products"] if int(p.get("id") or 0) == pid), None)
    if not product or product.get("status") == "off":
        raise HttpError(404, "商品不存在或已下架", "PRODUCT_NOT_FOUND")

    qty = max(1, min(50, integer(body.get("qty"), 1, 1, 50)))
    contact_name = text(body.get("contactName"), 40)
    contact_phone = text(body.get("contactPhone"), 40)
    game_account = text(body.get("gameAccount"), 120)
    server = text(body.get("server"), 60)
    note = text(body.get("note"), 500)

    if not contact_name:
        raise HttpError(400, "请填写称呼", "VALIDATION")
    if len(contact_phone) < 5:
        raise HttpError(400, "请填写手机号 / QQ / 微信，方便值班联系你", "VALIDATION")
    if not game_account:
        raise HttpError(400, "请填写游戏账号或角色 ID", "VALIDATION")

    stock = product.get("stock")
    if stock is not None and qty > stock:
        raise HttpError(400, "库存不够，先少买点或者问下客服", "NO_STOCK")

    amount = round(float(product.get("price") or 0) * qty, 2)
    no = order_no()
    order = {
        "id": "ord_" + no,
        "orderNo": no,
        "productId": product["id"],
        "productTitle": product.get("title"),
        "game": product.get("game"),
        "category": product.get("category"),
        "cover": product.get("cover"),
        "price": product.get("price"),
        "unit": product.get("unit"),
        "qty": qty,
        "amount": amount,
        "contactName": contact_name,
        "contactPhone": contact_phone,
        "gameAccount": game_account,
        "server": server,
        "note": note,
        "status": "pending",
        "payStatus": "unpaid",
        "payMethod": "",
        "payAt": "",
        "payTradeNo": "",
        "payProof": "",
        "payNote": "",
        "assigneeId": "",
        "assigneeName": "",
        "assignedAt": "",
        "supportNote": "",
        "progress": [],
        "createdAt": now_iso(),
        "updatedAt": now_iso(),
    }

    def mutate(db2):
        db2["orders"].insert(0, order)
        for p in db2["products"]:
            if int(p.get("id") or 0) == pid and p.get("stock") is not None:
                p["stock"] = max(0, int(p["stock"]) - qty)
        store.add_log(db2, "新订单 {} {}".format(no, product.get("title")))
        return order

    store.update(mutate)

    ok(ctx.h, {
        "orderNo": no,
        "amount": amount,
        "status": "pending",
        "productTitle": product.get("title"),
        "payStatus": "unpaid",
        "payment": _payment_public(db["settings"]),
        "kefu": db["settings"].get("kefu"),
        "tips": db["settings"].get("orderTips"),
    }, 201)


def track_order(ctx):
    db = store.load()
    no = (ctx.params.get("orderNo") or "").strip().upper()[:40]
    order = next((o for o in db["orders"] if str(o.get("orderNo")).upper() == no), None)
    if not order:
        raise HttpError(404, "没查到这个订单号，核对一下再试", "ORDER_NOT_FOUND")
    ok(ctx.h, {
        "orderNo": order.get("orderNo"),
        "productTitle": order.get("productTitle"),
        "game": order.get("game"),
        "cover": order.get("cover"),
        "qty": order.get("qty"),
        "amount": order.get("amount"),
        "status": order.get("status"),
        "contactName": order.get("contactName"),
        "contactPhone": mask_phone(order.get("contactPhone")),
        "gameAccount": order.get("gameAccount"),
        "server": order.get("server"),
        "note": order.get("note"),
        "payStatus": order.get("payStatus") or "unpaid",
        "payMethod": order.get("payMethod") or "",
        "payAt": order.get("payAt") or "",
        "payProof": order.get("payProof") or "",
        "payment": _payment_public(db["settings"]),
        "assigneeName": order.get("assigneeName") or "",
        "progress": [
            {"at": p.get("at"), "byName": p.get("byName") or "", "text": p.get("text") or ""}
            for p in (order.get("progress") or [])[-8:]
        ],
        "createdAt": order.get("createdAt"),
        "updatedAt": order.get("updatedAt"),
        "kefu": db["settings"].get("kefu"),
    })


# ---------------------------------------------------------------- 支付

MAGIC = {
    "image/png": ("png", b"\x89PNG"),
    "image/jpeg": ("jpg", b"\xff\xd8\xff"),
    "image/gif": ("gif", b"GIF"),
    "image/webp": ("webp", b"RIFF"),
}

PAY_ACTIONS = {"confirm": "paid", "refund": "refunded", "unpaid": "unpaid"}


def _save_image(data_url, limit=5 * 1024 * 1024, name_hint=""):
    data_url = str(data_url or "")
    if "," not in data_url or not data_url.startswith("data:"):
        raise HttpError(400, "图片数据不对", "BAD_IMAGE")
    header, payload = data_url.split(",", 1)
    mime = header[5:].split(";")[0].lower()
    if mime not in MAGIC:
        raise HttpError(400, "只支持 PNG / JPG / GIF / WEBP", "UNSUPPORTED_IMAGE")
    try:
        raw = base64.b64decode(payload, validate=True)
    except (binascii.Error, ValueError):
        raise HttpError(400, "图片数据损坏", "BAD_IMAGE")
    if len(raw) > limit:
        raise HttpError(413, "图片不要超过 {}MB".format(limit // (1024 * 1024)), "TOO_LARGE")
    ext, magic = MAGIC[mime]
    if len(raw) < 12 or not raw.startswith(magic):
        raise HttpError(400, "图片内容和格式不匹配", "BAD_IMAGE")
    name = "img_{}_{}.{}".format(hashlib.sha256(raw).hexdigest()[:16], uid()[:8], ext)
    Path(store.UPLOAD_DIR).mkdir(parents=True, exist_ok=True)
    (Path(store.UPLOAD_DIR) / name).write_bytes(raw)
    return "/uploads/" + name, len(raw)


def _find_order(db, no):
    no = str(no or "").strip().upper()[:40]
    return next((o for o in db["orders"] if str(o.get("orderNo")).upper() == no), None)


def _base_url(h):
    proto = (h.headers.get("X-Forwarded-Proto") or "http").split(",")[0].strip() or "http"
    host = h.headers.get("Host") or "127.0.0.1"
    return "{}://{}".format(proto, host)


def _epay_sign(params, key):
    pairs = []
    for k in sorted(params):
        if k in ("sign", "sign_type"):
            continue
        v = params[k]
        if v is None or str(v) == "":
            continue
        pairs.append("{}={}".format(k, v))
    return hashlib.md5(("&".join(pairs) + key).encode("utf-8")).hexdigest().lower()


def submit_proof(ctx):
    no = (ctx.params.get("orderNo") or "").strip().upper()
    cost = ctx.body.get("data")
    url, size = _save_image(cost, limit=3 * 1024 * 1024)
    holder = {}

    def mutate(db):
        order = _find_order(db, no)
        if not order:
            raise HttpError(404, "没查到这个订单号", "ORDER_NOT_FOUND")
        if order.get("payStatus") == "paid":
            raise HttpError(400, "这单已经确认到账了", "ALREADY_PAID")
        order["payStatus"] = "review"
        order["payMethod"] = order.get("payMethod") or "manual"
        order["payProof"] = url
        order["updatedAt"] = now_iso()
        store.add_log(db, "上传付款凭证 " + str(order.get("orderNo")))
        holder["o"] = order
        return order

    store.update(mutate)
    ok(ctx.h, {"orderNo": holder["o"].get("orderNo"), "payStatus": "review", "payProof": url, "size": size}, 201)


def pay_create(ctx):
    no = (ctx.params.get("orderNo") or "").strip().upper()
    db = store.load()
    order = _find_order(db, no)
    if not order:
        raise HttpError(404, "没查到这个订单号", "ORDER_NOT_FOUND")
    if order.get("payStatus") == "paid":
        raise HttpError(400, "这单已经付过了", "ALREADY_PAID")
    epay = ((db["settings"].get("payment") or {}).get("epay") or {})
    if not (epay.get("enabled") and epay.get("gateway") and epay.get("pid") and epay.get("key")):
        raise HttpError(400, "在线支付还没开通，用收款码付款", "PAY_DISABLED")

    base = _base_url(ctx.h)
    params = {
        "pid": str(epay.get("pid")),
        "type": str(epay.get("type") or "alipay"),
        "out_trade_no": order["orderNo"],
        "notify_url": base + "/api/pay/notify",
        "return_url": base + "/api/pay/return",
        "name": str(order.get("productTitle") or "护航服务")[:60],
        "money": "{:.2f}".format(float(order.get("amount") or 0)),
    }
    params["sign"] = _epay_sign(params, str(epay.get("key")))
    params["sign_type"] = "MD5"

    gateway = str(epay.get("gateway")).rstrip("/")
    url = gateway if "submit.php" in gateway else gateway + "/submit.php"
    ok(ctx.h, {"orderNo": order["orderNo"], "payUrl": url + "?" + urlencode(params)})


def epay_notify(ctx):
    params = dict(ctx.query)
    params.update({k: v for k, v in (ctx.form or {}).items() if v})
    db = store.load()
    epay = ((db["settings"].get("payment") or {}).get("epay") or {})
    if not epay.get("key"):
        ctx.h._send(200, b"fail", "text/plain; charset=utf-8")
        return
    if not _epay_verify(params, str(epay.get("key"))):
        ctx.h._send(200, b"fail", "text/plain; charset=utf-8")
        return
    if str(params.get("trade_status") or "") not in ("TRADE_SUCCESS", "TRADE_FINISHED"):
        ctx.h._send(200, b"fail", "text/plain; charset=utf-8")
        return
    no = str(params.get("out_trade_no") or "").strip().upper()
    holder = {}

    def mutate(db2):
        order = _find_order(db2, no)
        if not order:
            return None
        try:
            paid = float(params.get("money") or 0)
        except ValueError:
            return None
        if abs(paid - float(order.get("amount") or 0)) > 0.01:
            return None
        if order.get("payStatus") != "paid":
            order["payStatus"] = "paid"
            order["payMethod"] = "epay"
            order["payAt"] = now_iso()
            order["payTradeNo"] = str(params.get("trade_no") or "")
            order["updatedAt"] = now_iso()
            store.add_log(db2, "在线支付到账 " + order["orderNo"])
        holder["o"] = order
        return order

    store.update(mutate)
    body = b"success" if "o" in holder else b"fail"
    ctx.h._send(200, body, "text/plain; charset=utf-8")


def _epay_verify(params, key):
    sign = str(params.get("sign") or "").lower()
    if not sign:
        return False
    expected = _epay_sign(params, key)
    return hmac.compare_digest(sign, expected)


def epay_return(ctx):
    no = str(ctx.query.get("out_trade_no") or "").strip().upper()
    target = "/track.html" + ("?no=" + no if no else "")
    ctx.h._redirect(target)


def admin_order_payment(ctx):
    oid = ctx.params["id"]
    action = text(ctx.body.get("action"), 20)
    status = PAY_ACTIONS.get(action)
    if not status:
        raise HttpError(400, "操作不对", "VALIDATION")
    holder = {}

    def mutate(db):
        for o in db["orders"]:
            if o.get("id") == oid or o.get("orderNo") == oid:
                o["payStatus"] = status
                if status == "paid":
                    o["payAt"] = now_iso()
                    o["payMethod"] = o.get("payMethod") or "manual"
                if status == "unpaid":
                    o["payAt"] = ""
                    o["payTradeNo"] = ""
                o["updatedAt"] = now_iso()
                store.add_log(db, "订单 {} 付款状态 → {}".format(o.get("orderNo"), status))
                holder["o"] = o
                return o
        return None

    store.update(mutate)
    if "o" not in holder:
        raise HttpError(404, "订单不存在", "ORDER_NOT_FOUND")
    ok(ctx.h, holder["o"])


# ---------------------------------------------------------------- 后台

def _check_login_rate(ip):
    now = datetime.now().timestamp()
    record = _login_attempts.get(ip)
    if record and record[0] >= LOGIN_MAX and now < record[1]:
        raise HttpError(429, "登录太频繁，{} 秒后再试".format(int(record[1] - now)), "TOO_MANY_ATTEMPTS")
    if not record or now >= record[1]:
        _login_attempts[ip] = [0, now + LOGIN_WINDOW]


def _note_login_fail(ip):
    record = _login_attempts.setdefault(ip, [0, datetime.now().timestamp() + LOGIN_WINDOW])
    record[0] += 1


def _find_user(db, username):
    name = str(username or "").strip()
    return next((u for u in db.get("users", []) if str(u.get("username", "")).lower() == name.lower()), None)


def _user_by_id(db, uid):
    return next((u for u in db.get("users", []) if u.get("id") == uid), None)


def _sanitize_user(user):
    keys = ("id", "username", "name", "role", "phone", "share", "status", "createdAt", "lastLoginAt")
    return {k: user.get(k) for k in keys}


def _current(ctx):
    db = store.load()
    user = _user_by_id(db, (ctx.user or {}).get("uid"))
    if not user or user.get("status") == "off":
        raise HttpError(401, "账号已停用或不存在", "UNAUTHORIZED")
    return db, user


def login(ctx):
    _check_login_rate(ctx.ip)
    username = text(ctx.body.get("username"), 40)
    password = str(ctx.body.get("password") or "")[:100]
    if not username or not password:
        raise HttpError(400, "账号和密码都要填", "VALIDATION")

    db = store.load()
    user = _find_user(db, username)
    if not user or user.get("status") == "off" or not security.verify_password(
        password, user.get("passwordHash")
    ):
        _note_login_fail(ctx.ip)
        raise HttpError(401, "账号或密码不对", "BAD_CREDENTIALS")

    _login_attempts.pop(ctx.ip, None)

    def mutate(db2):
        target = _find_user(db2, username)
        target["lastLoginAt"] = now_iso()
        if target.get("role") == "owner":
            db2["admin"]["lastLoginAt"] = now_iso()
        store.add_log(db2, "{} 登录".format(target.get("name") or target.get("username")))

    store.update(mutate)
    public = _sanitize_user(_find_user(store.load(), username))
    ok(ctx.h, {
        "token": security.sign({
            "u": public["username"], "role": public["role"],
            "uid": public["id"], "name": public["name"],
        }),
        "expiresIn": security.TOKEN_TTL,
        "user": public,
        "role": public["role"],
        "admin": {"username": public["username"], "displayName": public["name"]},
    })


def me(ctx):
    db, user = _current(ctx)
    s = db["settings"]
    ok(ctx.h, {
        "id": user.get("id"),
        "username": user.get("username"),
        "name": user.get("name"),
        "displayName": user.get("name"),
        "role": user.get("role"),
        "share": user.get("share") or 0,
        "status": user.get("status"),
        "siteName": s.get("siteName"),
        "company": s.get("company"),
        "copyright": s.get("copyright"),
        "lastLoginAt": user.get("lastLoginAt"),
    })


def overview(ctx):
    db = store.load()
    orders = db["orders"]
    today = date.today().isoformat()
    done = [o for o in orders if o.get("status") == "done"]
    revenue = round(sum(float(o.get("amount") or 0) for o in done), 2)
    today_orders = sum(1 for o in orders if str(o.get("createdAt"))[:10] == today)

    series = []
    for i in range(6, -1, -1):
        day = (date.today() - timedelta(days=i)).isoformat()
        rows = [o for o in orders if str(o.get("createdAt"))[:10] == day]
        series.append({
            "date": day[5:],
            "count": len(rows),
            "amount": round(sum(float(o.get("amount") or 0) for o in rows), 2),
        })

    ok(ctx.h, {
        "totals": {
            "orders": len(orders),
            "pending": sum(1 for o in orders if o.get("status") == "pending"),
            "processing": sum(1 for o in orders if o.get("status") == "processing"),
            "done": len(done),
            "todayOrders": today_orders,
            "revenue": revenue,
            "products": len(db["products"]),
            "onSale": len(_visible(db)),
            "unpaid": sum(1 for o in orders if (o.get("payStatus") or "unpaid") == "unpaid"),
            "payReview": sum(1 for o in orders if o.get("payStatus") == "review"),
            "paid": sum(1 for o in orders if o.get("payStatus") == "paid"),
            "unassigned": sum(1 for o in orders if not o.get("assigneeId") and o.get("status") != "cancelled"),
            "boosters": sum(1 for u in db.get("users", []) if u.get("role") == "booster" and u.get("status") != "off"),
        },
        "series": series,
        "recentOrders": orders[:8],
        "logs": db["logs"][:6],
    })


def admin_products(ctx):
    db = store.load()
    q = (ctx.query.get("q") or "").strip().lower()
    game = (ctx.query.get("game") or "").strip()
    items = list(db["products"])
    if q:
        items = [p for p in items if q in " ".join(str(p.get(k) or "") for k in ("title", "game", "category")).lower()]
    if game and game != "全部":
        items = [p for p in items if p.get("game") == game]
    items.sort(key=lambda p: (p.get("sort") or 0, -(int(p.get("id") or 0))))
    ok(ctx.h, {"total": len(items), "items": items})


def _normalize_product(body, existing):
    base = dict(existing or {})
    title = text(body.get("title"), 120) or base.get("title")
    if not title:
        raise HttpError(400, "商品标题不能空", "VALIDATION")
    base.update({
        "title": title,
        "game": text(body.get("game"), 40) or base.get("game") or "其他",
        "category": text(body.get("category"), 40) or base.get("category") or "段位护航",
        "subtitle": text(body.get("subtitle"), 120),
        "cover": text(body.get("cover"), 500) or base.get("cover") or "/assets/img/cover-default.svg",
        "price": round(number(body.get("price"), base.get("price") or 0, 0, 999999), 2),
        "oldPrice": round(number(body.get("oldPrice"), base.get("oldPrice") or 0, 0, 999999), 2),
        "unit": text(body.get("unit"), 12) or base.get("unit") or "局",
        "tags": string_list(body.get("tags"), 6, 16),
        "desc": text(body.get("desc"), 4000),
        "includes": string_list(body.get("includes"), 8, 40),
        "stock": int(number(body.get("stock"), base.get("stock") if base.get("stock") is not None else 1, 0, 99999)),
        "sales": int(number(body.get("sales"), base.get("sales") or 0, 0, 9999999)),
        "status": "off" if body.get("status") == "off" else "on",
        "sort": int(number(body.get("sort"), base.get("sort") or 0, 0, 99999)),
        "updatedAt": now_iso(),
    })
    return base


def create_product(ctx):
    payload = _normalize_product(ctx.body, {"id": 0, "createdAt": now_iso(), "status": "on"})
    holder = {}

    def mutate(db):
        payload["id"] = store.next_product_id(db)
        db["products"].append(payload)
        store.add_log(db, "上架商品 " + payload["title"])
        holder["p"] = payload
        return payload

    store.update(mutate)
    ok(ctx.h, holder["p"], 201)


def update_product(ctx):
    pid = int(ctx.params["id"])
    holder = {}

    def mutate(db):
        for p in db["products"]:
            if int(p.get("id") or 0) == pid:
                p.update(_normalize_product(ctx.body, p))
                store.add_log(db, "更新商品 " + p["title"])
                holder["p"] = p
                return p
        return None

    store.update(mutate)
    if "p" not in holder:
        raise HttpError(404, "商品不存在", "PRODUCT_NOT_FOUND")
    ok(ctx.h, holder["p"])


def delete_product(ctx):
    pid = int(ctx.params["id"])
    holder = {}

    def mutate(db):
        for index, p in enumerate(db["products"]):
            if int(p.get("id") or 0) == pid:
                holder["p"] = db["products"].pop(index)
                store.add_log(db, "删除商品 " + holder["p"]["title"])
                return holder["p"]
        return None

    store.update(mutate)
    if "p" not in holder:
        raise HttpError(404, "商品不存在", "PRODUCT_NOT_FOUND")
    ok(ctx.h, {"id": pid})


def admin_orders(ctx):
    db = store.load()
    status = (ctx.query.get("status") or "all").strip()
    q = (ctx.query.get("q") or "").strip().lower()
    items = list(db["orders"])
    if status != "all":
        items = [o for o in items if o.get("status") == status]
    if q:
        def haystack(o):
            return " ".join(str(o.get(k) or "") for k in (
                "orderNo", "contactName", "contactPhone", "productTitle", "gameAccount"
            )).lower()
        items = [o for o in items if q in haystack(o)]
    ok(ctx.h, {"total": len(items), "items": items})


def update_order(ctx):
    oid = ctx.params["id"]
    status = text(ctx.body.get("status"), 20)
    if status and status not in ORDER_STATUS:
        raise HttpError(400, "订单状态不对", "VALIDATION")
    support_note = ctx.body.get("supportNote")
    holder = {}

    def mutate(db):
        for o in db["orders"]:
            if o.get("id") == oid or o.get("orderNo") == oid:
                if status:
                    o["status"] = status
                if isinstance(support_note, str):
                    o["supportNote"] = text(support_note, 500)
                o["updatedAt"] = now_iso()
                store.add_log(db, "订单 {} → {}".format(o.get("orderNo"), status or "更新备注"))
                holder["o"] = o
                return o
        return None

    store.update(mutate)
    if "o" not in holder:
        raise HttpError(404, "订单不存在", "ORDER_NOT_FOUND")
    ok(ctx.h, holder["o"])


def delete_order(ctx):
    oid = ctx.params["id"]
    holder = {}

    def mutate(db):
        for index, o in enumerate(db["orders"]):
            if o.get("id") == oid or o.get("orderNo") == oid:
                holder["o"] = db["orders"].pop(index)
                store.add_log(db, "删除订单 " + str(holder["o"].get("orderNo")))
                return holder["o"]
        return None

    store.update(mutate)
    if "o" not in holder:
        raise HttpError(404, "订单不存在", "ORDER_NOT_FOUND")
    ok(ctx.h, {"id": oid})


def get_settings(ctx):
    settings = json.loads(json.dumps(store.load()["settings"], ensure_ascii=False))
    if _role(ctx) != "owner":
        payment = settings.get("payment") or {}
        epay = payment.get("epay") or {}
        epay["key"] = ""
        payment["epay"] = epay
        settings["payment"] = payment
    ok(ctx.h, settings)


def update_settings(ctx):
    body = ctx.body
    holder = {}

    def mutate(db):
        s = db["settings"]
        for key, limit in (("siteName", 40), ("company", 40), ("slogan", 120),
                           ("subSlogan", 160), ("notice", 300), ("copyright", 80),
                           ("icp", 80), ("adminNotice", 300), ("orderTips", 300)):
            if isinstance(body.get(key), str):
                s[key] = text(body.get(key), limit)
        if isinstance(body.get("kefu"), dict):
            s["kefu"] = {
                "qq": text(body["kefu"].get("qq"), 40),
                "wechat": text(body["kefu"].get("wechat"), 40),
                "phone": text(body["kefu"].get("phone"), 40),
                "hours": text(body["kefu"].get("hours"), 60),
            }
        if isinstance(body.get("payment"), dict) and _role(ctx) == "owner":
            pay = body["payment"]
            manual = pay.get("manual") if isinstance(pay.get("manual"), dict) else {}
            epay = pay.get("epay") if isinstance(pay.get("epay"), dict) else {}
            s["payment"] = {
                "manual": {
                    "enabled": bool(manual.get("enabled", True)),
                    "wechatQr": text(manual.get("wechatQr"), 300),
                    "alipayQr": text(manual.get("alipayQr"), 300),
                    "note": text(manual.get("note"), 200),
                },
                "epay": {
                    "enabled": bool(epay.get("enabled")),
                    "gateway": text(epay.get("gateway"), 200).rstrip("/"),
                    "pid": text(epay.get("pid"), 40),
                    "key": text(epay.get("key"), 120),
                    "type": epay.get("type") if epay.get("type") in ("alipay", "wxpay") else "alipay",
                    "name": text(epay.get("name"), 20) or "在线支付",
                },
            }
        if isinstance(body.get("heroStats"), list):
            s["heroStats"] = [
                {"label": text(item.get("label"), 20), "value": text(item.get("value"), 12),
                 "suffix": text(item.get("suffix"), 6)}
                for item in body["heroStats"][:4] if isinstance(item, dict)
            ]
        store.add_log(db, "更新站点设置")
        holder["s"] = s

    store.update(mutate)
    ok(ctx.h, holder["s"])


def upload(ctx):
    url, size = _save_image(ctx.body.get("data"), limit=5 * 1024 * 1024)

    def mutate(db):
        store.add_log(db, "上传图片 " + url[9:])

    store.update(mutate)
    ok(ctx.h, {"url": url, "size": size}, 201)


def change_password(ctx):
    old = str(ctx.body.get("oldPassword") or "")
    new = str(ctx.body.get("newPassword") or "")
    if len(new) < 6:
        raise HttpError(400, "新密码至少 6 位", "VALIDATION")
    db = store.load()
    user = _user_by_id(db, (ctx.user or {}).get("uid"))
    if not user or not security.verify_password(old, user.get("passwordHash")):
        raise HttpError(401, "原密码不对", "BAD_CREDENTIALS")

    def mutate(db2):
        target = _user_by_id(db2, user["id"])
        target["passwordHash"] = security.hash_password(new)
        if target.get("role") == "owner":
            db2["admin"]["passwordHash"] = target["passwordHash"]
        store.add_log(db2, "{} 修改密码".format(target.get("name") or target.get("username")))

    store.update(mutate)
    ok(ctx.h, {"changed": True})


# ---------------------------------------------------------------- 多角色与工作台

import re

ROLE_NAMES = {"owner": "董事长", "manager": "管事", "support": "客服", "booster": "打手"}
USERNAME_RE = re.compile(r"^[a-zA-Z0-9_]{3,20}$")


def _role(ctx):
    return (ctx.user or {}).get("role") or ""


def _staff_view(order, mask=False):
    keys = (
        "id", "orderNo", "productTitle", "game", "category", "qty", "amount",
        "status", "payStatus", "payMethod", "payAt", "contactName", "contactPhone",
        "gameAccount", "server", "note", "supportNote", "assigneeId", "assigneeName",
        "assignedAt", "createdAt", "updatedAt", "progress",
    )
    out = {k: order.get(k) for k in keys}
    if mask:
        out["contactPhone"] = mask_phone(order.get("contactPhone"))
    return out


def staff_orders(ctx):
    db, user = _current(ctx)
    role = user.get("role")
    scope = (ctx.query.get("scope") or "mine").strip()
    status = (ctx.query.get("status") or "all").strip()
    q = (ctx.query.get("q") or "").strip().lower()

    items = list(db["orders"])
    if role == "booster":
        if scope == "pool":
            items = [o for o in items if not o.get("assigneeId") and o.get("status") == "pending"
                     and (o.get("payStatus") or "unpaid") == "paid"]
        else:
            items = [o for o in items if o.get("assigneeId") == user.get("id")]
    if status != "all":
        items = [o for o in items if o.get("status") == status]
    if q:
        def hay(o):
            return " ".join(str(o.get(k) or "") for k in (
                "orderNo", "productTitle", "gameAccount", "contactName", "assigneeName"
            )).lower()
        items = [o for o in items if q in hay(o)]

    ok(ctx.h, {
        "total": len(items),
        "items": [_staff_view(o, mask=(role == "booster")) for o in items],
        "role": role,
    })


def _staff_order_or_403(db, ctx, oid):
    for o in db["orders"]:
        if o.get("id") == oid or o.get("orderNo") == oid:
            if _role(ctx) == "booster" and o.get("assigneeId") != (ctx.user or {}).get("uid"):
                raise HttpError(403, "这单不是派给你的", "FORBIDDEN")
            return o
    raise HttpError(404, "订单不存在", "ORDER_NOT_FOUND")


def staff_order_detail(ctx):
    db, user = _current(ctx)
    order = _staff_order_or_403(db, ctx, ctx.params["id"])
    ok(ctx.h, _staff_view(order, mask=(user.get("role") == "booster")))


def staff_claim(ctx):
    db, user = _current(ctx)
    if user.get("role") != "booster":
        raise HttpError(403, "只有打手可以接单池里的单", "FORBIDDEN")
    oid = ctx.params["id"]
    holder = {}

    def mutate(db2):
        for o in db2["orders"]:
            if o.get("id") != oid and o.get("orderNo") != oid:
                continue
            if o.get("assigneeId") and o.get("assigneeId") != user.get("id"):
                raise HttpError(400, "这单已经被别人接了", "TAKEN")
            if o.get("status") != "pending":
                raise HttpError(400, "这单已经在进行或结束了", "BAD_STATUS")
            if (o.get("payStatus") or "unpaid") != "paid":
                raise HttpError(400, "客户还没付款，先别接", "NOT_PAID")
            o["assigneeId"] = user.get("id")
            o["assigneeName"] = user.get("name")
            o["assignedAt"] = now_iso()
            o["status"] = "processing"
            o["updatedAt"] = now_iso()
            store.add_log(db2, "{} 接了订单 {}".format(user.get("name"), o.get("orderNo")))
            holder["o"] = o
            return o
        raise HttpError(404, "订单不存在", "ORDER_NOT_FOUND")

    store.update(mutate)
    ok(ctx.h, _staff_view(holder["o"], mask=True))


def staff_progress(ctx):
    db, user = _current(ctx)
    oid = ctx.params["id"]
    text_value = text(ctx.body.get("text"), 200)
    if not text_value:
        raise HttpError(400, "写一句进度说明", "VALIDATION")
    image = ""
    if ctx.body.get("image"):
        image, _ = _save_image(ctx.body.get("image"), limit=3 * 1024 * 1024, name_hint="progress_")
    holder = {}

    def mutate(db2):
        order = None
        for o in db2["orders"]:
            if o.get("id") == oid or o.get("orderNo") == oid:
                order = o
                break
        if not order:
            raise HttpError(404, "订单不存在", "ORDER_NOT_FOUND")
        role = user.get("role")
        if role == "booster" and order.get("assigneeId") != user.get("id"):
            raise HttpError(403, "这单不是派给你的", "FORBIDDEN")
        if role not in ("owner", "manager", "support", "booster"):
            raise HttpError(403, "没有权限", "FORBIDDEN")
        entry = {
            "at": now_iso(), "by": user.get("id"), "byName": user.get("name"),
            "text": text_value, "image": image,
        }
        order.setdefault("progress", []).append(entry)
        order["progress"] = order["progress"][-30:]
        if role == "booster" and order.get("status") == "pending":
            order["status"] = "processing"
        order["updatedAt"] = now_iso()
        store.add_log(db2, "{} 更新订单 {} 进度".format(user.get("name"), order.get("orderNo")))
        holder["o"] = order
        return order

    store.update(mutate)
    ok(ctx.h, _staff_view(holder["o"], mask=(user.get("role") == "booster")), 201)


def staff_status(ctx):
    db, user = _current(ctx)
    status = text(ctx.body.get("status"), 20)
    if status not in ("processing", "done"):
        raise HttpError(400, "只能改成进行中或已完成", "VALIDATION")
    oid = ctx.params["id"]
    holder = {}

    def mutate(db2):
        order = _staff_order_or_403(db2, ctx, oid)
        if user.get("role") == "booster" and order.get("assigneeId") != user.get("id"):
            raise HttpError(403, "这单不是派给你的", "FORBIDDEN")
        order["status"] = status
        order["updatedAt"] = now_iso()
        store.add_log(db2, "{} 把订单 {} 标记为 {}".format(user.get("name"), order.get("orderNo"), status))
        holder["o"] = order
        return order

    store.update(mutate)
    ok(ctx.h, _staff_view(holder["o"], mask=(user.get("role") == "booster")))


def staff_summary(ctx):
    db, user = _current(ctx)
    role = user.get("role")
    orders = db["orders"]
    month = date.today().isoformat()[:7]

    if role == "booster":
        mine = [o for o in orders if o.get("assigneeId") == user.get("id")]
        done = [o for o in mine if o.get("status") == "done"]
        month_done = [o for o in done if str(o.get("updatedAt"))[:7] == month]
        share = int(user.get("share") or 0)
        amount = round(sum(float(o.get("amount") or 0) for o in month_done), 2)
        ok(ctx.h, {
            "role": role,
            "assigned": len(mine),
            "processing": sum(1 for o in mine if o.get("status") == "processing"),
            "done": len(done),
            "monthDone": len(month_done),
            "monthAmount": amount,
            "share": share,
            "estimate": round(amount * share / 100.0, 2),
            "pool": sum(1 for o in orders if not o.get("assigneeId") and o.get("status") == "pending"
                        and (o.get("payStatus") or "unpaid") == "paid"),
        })
        return

    today = date.today().isoformat()
    ok(ctx.h, {
        "role": role,
        "total": len(orders),
        "pending": sum(1 for o in orders if o.get("status") == "pending"),
        "processing": sum(1 for o in orders if o.get("status") == "processing"),
        "doneToday": sum(1 for o in orders if o.get("status") == "done" and str(o.get("updatedAt"))[:10] == today),
        "unpaid": sum(1 for o in orders if (o.get("payStatus") or "unpaid") == "unpaid"),
        "review": sum(1 for o in orders if o.get("payStatus") == "review"),
        "unassigned": sum(1 for o in orders if not o.get("assigneeId") and o.get("status") != "cancelled"),
        "boosters": sum(1 for u in db["users"] if u.get("role") == "booster" and u.get("status") != "off"),
    })


def admin_users(ctx):
    db, user = _current(ctx)
    role = user.get("role")
    items = list(db["users"])
    if role == "manager":
        items = [u for u in items if u.get("role") in ("support", "booster")]
    items.sort(key=lambda u: ({"owner": 0, "manager": 1, "support": 2, "booster": 3}.get(u.get("role"), 9), u.get("createdAt") or ""))
    ok(ctx.h, {"total": len(items), "items": [_sanitize_user(u) for u in items]})


def _check_manage_user(actor, target_role, new_role=None):
    if actor.get("role") == "owner":
        return
    if target_role not in ("support", "booster") or (new_role and new_role not in ("support", "booster")):
        raise HttpError(403, "管事只能管理客服和打手账号", "FORBIDDEN")


def create_user(ctx):
    db, actor = _current(ctx)
    body = ctx.body
    username = text(body.get("username"), 20)
    if not USERNAME_RE.match(username):
        raise HttpError(400, "账号只能是 3-20 位字母、数字或下划线", "VALIDATION")
    role = text(body.get("role"), 10)
    if role not in ROLE_NAMES:
        raise HttpError(400, "角色不对", "VALIDATION")
    _check_manage_user(actor, role)
    if role == "owner":
        raise HttpError(400, "董事长账号只能有一个，别新建", "VALIDATION")
    password = str(body.get("password") or "")
    if len(password) < 6:
        raise HttpError(400, "密码至少 6 位", "VALIDATION")
    if _find_user(db, username):
        raise HttpError(400, "这个账号名已经有人用了", "USERNAME_TAKEN")

    payload = {
        "id": "usr_" + uid()[:8],
        "username": username,
        "name": text(body.get("name"), 30) or username,
        "role": role,
        "phone": text(body.get("phone"), 30),
        "share": int(number(body.get("share"), 60 if role == "booster" else 0, 0, 100)),
        "status": "on" if body.get("status") != "off" else "off",
        "passwordHash": security.hash_password(password),
        "createdAt": now_iso(),
        "lastLoginAt": "",
    }
    holder = {}

    def mutate(db2):
        db2["users"].append(payload)
        store.add_log(db2, "{} 新建{}账号 {}".format(actor.get("name"), ROLE_NAMES[role], username))
        holder["u"] = payload
        return payload

    store.update(mutate)
    ok(ctx.h, _sanitize_user(holder["u"]), 201)


def update_user(ctx):
    db, actor = _current(ctx)
    target = _user_by_id(db, ctx.params["id"])
    if not target:
        raise HttpError(404, "账号不存在", "USER_NOT_FOUND")
    new_role = text(ctx.body.get("role"), 10) or target.get("role")
    _check_manage_user(actor, target.get("role"), new_role)
    if target.get("role") == "owner" and actor.get("role") != "owner":
        raise HttpError(403, "董事长账号不能动", "FORBIDDEN")
    if new_role not in ROLE_NAMES:
        raise HttpError(400, "角色不对", "VALIDATION")
    if target.get("role") == "owner" and new_role != "owner":
        raise HttpError(400, "不能把董事长改成别的角色", "VALIDATION")

    def mutate(db2):
        user = _user_by_id(db2, target["id"])
        if "name" in ctx.body:
            user["name"] = text(ctx.body.get("name"), 30) or user.get("username")
        if "phone" in ctx.body:
            user["phone"] = text(ctx.body.get("phone"), 30)
        if "role" in ctx.body:
            user["role"] = new_role
        if "share" in ctx.body:
            user["share"] = int(number(ctx.body.get("share"), user.get("share") or 0, 0, 100))
        if "status" in ctx.body:
            if user.get("role") == "owner" and ctx.body.get("status") == "off":
                raise HttpError(400, "不能停用董事长账号", "VALIDATION")
            user["status"] = "on" if ctx.body.get("status") != "off" else "off"
        password = str(ctx.body.get("password") or "")
        if password:
            if len(password) < 6:
                raise HttpError(400, "新密码至少 6 位", "VALIDATION")
            user["passwordHash"] = security.hash_password(password)
        store.add_log(db2, "{} 更新账号 {}".format(actor.get("name"), user.get("username")))
        return _sanitize_user(user)

    result = store.update(mutate)
    ok(ctx.h, result)


def delete_user(ctx):
    db, actor = _current(ctx)
    target = _user_by_id(db, ctx.params["id"])
    if not target:
        raise HttpError(404, "账号不存在", "USER_NOT_FOUND")
    if target.get("id") == actor.get("id"):
        raise HttpError(400, "不能删自己", "VALIDATION")
    if target.get("role") == "owner":
        raise HttpError(400, "董事长账号不能删", "VALIDATION")
    holder = {}

    def mutate(db2):
        for index, u in enumerate(db2["users"]):
            if u.get("id") == target["id"]:
                holder["u"] = db2["users"].pop(index)
                break
        for o in db2["orders"]:
            if o.get("assigneeId") == target["id"] and o.get("status") != "done":
                o["assigneeId"] = ""
                o["assigneeName"] = ""
        store.add_log(db2, "删除账号 {}".format(target.get("username")))

    store.update(mutate)
    ok(ctx.h, {"id": target["id"]})


def admin_logs(ctx):
    db, user = _current(ctx)
    ok(ctx.h, {"items": db.get("logs", [])[:80]})


def assign_order(ctx):
    db, actor = _current(ctx)
    oid = ctx.params["id"]
    uid_value = text(ctx.body.get("userId"), 40)
    target = _user_by_id(db, uid_value) if uid_value else None
    if uid_value and (not target or target.get("role") != "booster" or target.get("status") == "off"):
        raise HttpError(400, "只能派给在岗的打手", "VALIDATION")
    holder = {}

    def mutate(db2):
        for o in db2["orders"]:
            if o.get("id") != oid and o.get("orderNo") != oid:
                continue
            if target:
                o["assigneeId"] = target["id"]
                o["assigneeName"] = target["name"]
                o["assignedAt"] = now_iso()
                if o.get("status") == "pending" and (o.get("payStatus") or "unpaid") == "paid":
                    o["status"] = "processing"
            else:
                o["assigneeId"] = ""
                o["assigneeName"] = ""
                o["assignedAt"] = ""
            o["updatedAt"] = now_iso()
            store.add_log(db2, "{} 派单 {} → {}".format(
                actor.get("name"), o.get("orderNo"), target.get("name") if target else "未分配"))
            holder["o"] = o
            return o
        raise HttpError(404, "订单不存在", "ORDER_NOT_FOUND")

    store.update(mutate)
    ok(ctx.h, holder["o"])
