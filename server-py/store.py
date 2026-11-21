"""数据层：一个 db.json，读写都走锁 + 临时文件替换，避免掉电写坏。

无定电竞护航系统 · Python 后端
"""

import json
import os
import shutil
import threading
from pathlib import Path

from security import hash_password
from util import now_iso, uid

ROOT = Path(__file__).resolve().parent.parent
DATA_DIR = Path(os.environ.get("DATA_DIR") or (ROOT / "data")).resolve()
DB_FILE = DATA_DIR / "db.json"
UPLOAD_DIR = Path(os.environ.get("UPLOAD_DIR") or (DATA_DIR / "uploads")).resolve()

ADMIN_USER = os.environ.get("ADMIN_USER", "wunding")
ADMIN_PASS = os.environ.get("ADMIN_PASS", "wunding2026")

_lock = threading.RLock()
_cache = None

SEED_FILE = ROOT / "seed.json"


def _load_seed():
    """默认商品和文案放在仓库根目录的 seed.json，Python 和 Go 两端共用。"""
    try:
        with open(SEED_FILE, encoding="utf-8") as handle:
            data = json.load(handle)
        if isinstance(data, dict) and isinstance(data.get("products"), list):
            return data
    except (OSError, ValueError):
        pass
    return {"products": [], "settings": {}}


SEED = _load_seed()

DEFAULT_SETTINGS = {
    "siteName": "无定电竞护航",
    "company": "无定科技",
    "slogan": "无定电竞护航",
    "subSlogan": "工作室直营的上分护航",
    "notice": "",
    "heroStats": [],
    "kefu": {"qq": "", "wechat": "", "phone": "", "hours": ""},
    "copyright": "© 2026 无定科技 版权所有",
    "icp": "",
    "adminNotice": "",
    "orderTips": "",
}
DEFAULT_SETTINGS.update(SEED.get("settings") or {})


def _seed_products():
    now = now_iso()
    products = []
    for index, row in enumerate(SEED.get("products", []), start=1):
        product = dict(row)
        product["id"] = index
        product.setdefault("status", "on")
        product.setdefault("sort", index)
        product.setdefault("stock", 10)
        product.setdefault("sales", 0)
        product["createdAt"] = now
        product["updatedAt"] = now
        products.append(product)
    return products


def default_db():
    return {
        "meta": {"version": 2, "brand": "无定科技", "createdAt": now_iso()},
        "admin": {
            "username": ADMIN_USER,
            "passwordHash": hash_password(ADMIN_PASS),
            "displayName": "无定科技 · 运营",
            "lastLoginAt": None,
        },
        "settings": json.loads(json.dumps(DEFAULT_SETTINGS, ensure_ascii=False)),
        "products": _seed_products(),
        "orders": [],
        "logs": [{"id": uid("log_"), "at": now_iso(), "action": "初始化数据"}],
    }


def _normalize(db):
    if not isinstance(db, dict):
        return default_db()
    db.setdefault("meta", {"version": 2, "brand": "无定科技", "createdAt": now_iso()})
    settings = json.loads(json.dumps(DEFAULT_SETTINGS, ensure_ascii=False))
    if isinstance(db.get("settings"), dict):
        settings.update(db["settings"])
        if isinstance(db["settings"].get("kefu"), dict):
            settings["kefu"] = dict(DEFAULT_SETTINGS["kefu"], **db["settings"]["kefu"])
    db["settings"] = settings
    if not isinstance(db.get("products"), list):
        db["products"] = _seed_products()
    if not isinstance(db.get("orders"), list):
        db["orders"] = []
    if not isinstance(db.get("logs"), list):
        db["logs"] = []
    if not isinstance(db.get("admin"), dict):
        db["admin"] = default_db()["admin"]
    return db


def save(db):
    DATA_DIR.mkdir(parents=True, exist_ok=True)
    UPLOAD_DIR.mkdir(parents=True, exist_ok=True)
    tmp = DB_FILE.with_suffix(".json.tmp")
    with open(tmp, "w", encoding="utf-8") as handle:
        json.dump(db, handle, ensure_ascii=False, indent=2)
        handle.flush()
        os.fsync(handle.fileno())
    os.replace(tmp, DB_FILE)
    global _cache
    _cache = db
    return db


def load(force=False):
    global _cache
    with _lock:
        if _cache is not None and not force:
            return _cache
        DATA_DIR.mkdir(parents=True, exist_ok=True)
        if not DB_FILE.exists():
            _cache = default_db()
            save(_cache)
            return _cache
        try:
            with open(DB_FILE, encoding="utf-8") as handle:
                _cache = _normalize(json.load(handle))
        except (json.JSONDecodeError, OSError):
            backup = DB_FILE.with_suffix(".json.broken")
            try:
                shutil.copyfile(DB_FILE, backup)
            except OSError:
                pass
            _cache = default_db()
            save(_cache)
        return _cache


def update(mutator):
    with _lock:
        db = load()
        result = mutator(db)
        db.setdefault("meta", {})["updatedAt"] = now_iso()
        save(db)
        return result


def next_product_id(db):
    ids = [int(p.get("id") or 0) for p in db.get("products", [])]
    return (max(ids) if ids else 0) + 1


def add_log(db, action):
    db.setdefault("logs", []).insert(0, {"id": uid("log_"), "at": now_iso(), "action": action})
    del db["logs"][200:]
