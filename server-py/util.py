"""请求解析、校验和响应工具。

无定电竞护航系统 · 后端（Python 版）
"""

import json
import secrets
from datetime import datetime
from http import HTTPStatus

MAX_BODY = 256 * 1024
MAX_UPLOAD = 8 * 1024 * 1024


class HttpError(Exception):
    """带状态码的业务错误，交给入口统一转成 JSON 响应。"""

    def __init__(self, status, message, code="ERROR"):
        super().__init__(message)
        self.status = int(status)
        self.message = message
        self.code = code


def now_iso():
    return datetime.now().astimezone().isoformat(timespec="seconds")


def uid(prefix=""):
    return prefix + secrets.token_hex(8)


def text(value, limit=2000):
    if not isinstance(value, str):
        return ""
    return value.strip()[:limit]


def number(value, default=0, minimum=-1e9, maximum=1e9):
    try:
        n = float(value)
    except (TypeError, ValueError):
        return default
    if n != n:  # NaN
        return default
    return max(minimum, min(maximum, n))


def integer(value, default=0, minimum=0, maximum=10 ** 9):
    return int(round(number(value, default, minimum, maximum)))


def flag(value, default=False):
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        if value.lower() in ("1", "true", "yes", "on"):
            return True
        if value.lower() in ("0", "false", "no", "off"):
            return False
    return default


def string_list(value, max_items=12, max_len=24):
    if not isinstance(value, list):
        return []
    out = []
    for item in value[:max_items]:
        if isinstance(item, str) and item.strip():
            out.append(item.strip()[:max_len])
    return out


def mask_phone(value):
    value = str(value or "")
    if len(value) < 7:
        return "***" if value else ""
    return value[:3] + "****" + value[-4:]


def order_no():
    stamp = datetime.now().strftime("%Y%m%d")
    return "WD" + stamp + str(secrets.randbelow(900000) + 100000)


def json_response(handler, status, payload, extra_headers=None):
    body = json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    handler.send_response(status)
    handler.send_header("Content-Type", "application/json; charset=utf-8")
    handler.send_header("Content-Length", str(len(body)))
    handler.send_header("Cache-Control", "no-store")
    handler.send_header("X-Content-Type-Options", "nosniff")
    for key, value in (extra_headers or {}).items():
        handler.send_header(key, value)
    handler.end_headers()
    if handler.command != "HEAD":
        handler.wfile.write(body)


def ok(handler, data, status=HTTPStatus.OK):
    json_response(handler, status, {"ok": True, "data": data})


def fail(handler, status, message, code="ERROR"):
    json_response(handler, status, {"ok": False, "error": {"message": message, "code": code}})
