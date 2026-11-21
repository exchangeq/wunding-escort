"""接口实现：前台 / 后台共用一份逻辑，Python 与 Go 版返回结构完全一致。"""

import base64
import binascii
import hashlib
from datetime import date, datetime, timedelta
from pathlib import Path

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


def site(ctx):
    db = store.load()
    items = _visible(db)
    games = {}
    for p in items:
        games[p.get("game") or "其他"] = games.get(p.get("game") or "其他", 0) + 1
    categories = sorted({p.get("category") for p in items if p.get("category")})
    ok(ctx.h, {
        "settings": db["settings"],
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
        "createdAt": order.get("createdAt"),
        "updatedAt": order.get("updatedAt"),
        "kefu": db["settings"].get("kefu"),
    })


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


def login(ctx):
    _check_login_rate(ctx.ip)
    username = text(ctx.body.get("username"), 40)
    password = str(ctx.body.get("password") or "")[:100]
    if not username or not password:
        raise HttpError(400, "账号和密码都要填", "VALIDATION")

    db = store.load()
    admin = db["admin"]
    if username.lower() != str(admin.get("username", "")).lower() or not security.verify_password(
        password, admin.get("passwordHash")
    ):
        _note_login_fail(ctx.ip)
        raise HttpError(401, "账号或密码不对", "BAD_CREDENTIALS")

    _login_attempts.pop(ctx.ip, None)

    def mutate(db2):
        db2["admin"]["lastLoginAt"] = now_iso()
        store.add_log(db2, "管理员登录")

    store.update(mutate)
    ok(ctx.h, {
        "token": security.sign({"u": admin.get("username"), "role": "admin"}),
        "expiresIn": security.TOKEN_TTL,
        "admin": {"username": admin.get("username"), "displayName": admin.get("displayName")},
    })


def me(ctx):
    db = store.load()
    s = db["settings"]
    ok(ctx.h, {
        "username": db["admin"].get("username"),
        "displayName": db["admin"].get("displayName"),
        "siteName": s.get("siteName"),
        "company": s.get("company"),
        "copyright": s.get("copyright"),
        "lastLoginAt": db["admin"].get("lastLoginAt"),
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
    if status not in ORDER_STATUS:
        raise HttpError(400, "订单状态不对", "VALIDATION")
    holder = {}

    def mutate(db):
        for o in db["orders"]:
            if o.get("id") == oid or o.get("orderNo") == oid:
                o["status"] = status
                o["updatedAt"] = now_iso()
                store.add_log(db, "订单 {} → {}".format(o.get("orderNo"), status))
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
    ok(ctx.h, store.load()["settings"])


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


MAGIC = {
    "image/png": ("png", b"\x89PNG"),
    "image/jpeg": ("jpg", b"\xff\xd8\xff"),
    "image/gif": ("gif", b"GIF"),
    "image/webp": ("webp", b"RIFF"),
}


def upload(ctx):
    data_url = str(ctx.body.get("data") or "")
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
    if len(raw) > 5 * 1024 * 1024:
        raise HttpError(413, "图片不要超过 5MB", "TOO_LARGE")
    ext, magic = MAGIC[mime]
    if len(raw) < 12 or not raw.startswith(magic):
        raise HttpError(400, "图片内容和格式不匹配", "BAD_IMAGE")

    name = "img_{}_{}.{}".format(hashlib.sha256(raw).hexdigest()[:16], uid()[:8], ext)
    store.UPLOAD_DIR.mkdir(parents=True, exist_ok=True)
    (Path(store.UPLOAD_DIR) / name).write_bytes(raw)

    def mutate(db):
        store.add_log(db, "上传图片 " + name)

    store.update(mutate)
    ok(ctx.h, {"url": "/uploads/" + name, "size": len(raw)}, 201)


def change_password(ctx):
    old = str(ctx.body.get("oldPassword") or "")
    new = str(ctx.body.get("newPassword") or "")
    if len(new) < 6:
        raise HttpError(400, "新密码至少 6 位", "VALIDATION")
    db = store.load()
    if not security.verify_password(old, db["admin"].get("passwordHash")):
        raise HttpError(401, "原密码不对", "BAD_CREDENTIALS")

    def mutate(db2):
        db2["admin"]["passwordHash"] = security.hash_password(new)
        store.add_log(db2, "管理员改密码")

    store.update(mutate)
    ok(ctx.h, {"changed": True})
