"""入口：路由、鉴权、静态文件，全部只用标准库，Termux 里直接 python3 app.py 就能跑。

无定电竞护航系统 · Python 后端
"""

import json
import mimetypes
import os
import posixpath
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, unquote, urlparse

import api
import security
import store
from util import MAX_UPLOAD, HttpError, fail, json_response, ok

ROOT = Path(__file__).resolve().parent.parent
WEB_ROOT = Path(os.environ.get("WEB_DIR") or (ROOT / "web")).resolve()
PORT = int(os.environ.get("PORT") or 8080)
HOST = os.environ.get("HOST") or "0.0.0.0"

PUBLIC = "public"
AUTH = "admin"

ROUTES = [
    ("GET", "/api/health", "health", PUBLIC),
    ("GET", "/api/site", "site", PUBLIC),
    ("GET", "/api/products", "products", PUBLIC),
    ("GET", "/api/products/{id}", "product_detail", PUBLIC),
    ("POST", "/api/orders", "create_order", PUBLIC),
    ("GET", "/api/orders/{orderNo}", "track_order", PUBLIC),
    ("POST", "/api/admin/login", "login", PUBLIC),
    ("GET", "/api/admin/me", "me", AUTH),
    ("GET", "/api/admin/overview", "overview", AUTH),
    ("GET", "/api/admin/products", "admin_products", AUTH),
    ("POST", "/api/admin/products", "create_product", AUTH),
    ("PUT", "/api/admin/products/{id}", "update_product", AUTH),
    ("DELETE", "/api/admin/products/{id}", "delete_product", AUTH),
    ("GET", "/api/admin/orders", "admin_orders", AUTH),
    ("PUT", "/api/admin/orders/{id}", "update_order", AUTH),
    ("DELETE", "/api/admin/orders/{id}", "delete_order", AUTH),
    ("GET", "/api/admin/settings", "get_settings", AUTH),
    ("PUT", "/api/admin/settings", "update_settings", AUTH),
    ("POST", "/api/admin/upload", "upload", AUTH),
    ("POST", "/api/admin/password", "change_password", AUTH),
]


class Ctx:
    def __init__(self, handler):
        self.h = handler
        self.params = {}
        self.query = {}
        self.body = {}
        self.ip = handler.client_ip
        self.user = None


def _match(method, path):
    parts = [unquote(p) for p in path.split("/") if p]
    for http_method, pattern, name, scope in ROUTES:
        if http_method != method:
            continue
        segs = pattern.strip("/").split("/")
        if len(segs) != len(parts):
            continue
        params = {}
        for seg, actual in zip(segs, parts):
            if seg.startswith("{") and seg.endswith("}"):
                params[seg[1:-1]] = actual
            elif seg != actual:
                break
        else:
            return name, params, scope
    return None, None, None


class Handler(BaseHTTPRequestHandler):
    server_version = "wunding-escort/2.0"
    protocol_version = "HTTP/1.1"

    # ---------- 基础 ----------
    @property
    def client_ip(self):
        forwarded = (self.headers.get("X-Forwarded-For") or "").split(",")[0].strip()
        return forwarded or self.client_address[0]

    def log_message(self, fmt, *args):  # 控制台只保留一行访问日志
        sys.stderr.write("[access] %s %s\n" % (self.client_ip, fmt % args))

    def _send(self, status, payload, content_type="application/json; charset=utf-8"):
        body = payload if isinstance(payload, bytes) else json.dumps(
            payload, ensure_ascii=False, separators=(",", ":")
        ).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.send_header("X-Content-Type-Options", "nosniff")
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)

    # ---------- 方法 ----------
    def do_OPTIONS(self):
        self.send_response(204)
        self.send_header("Allow", "GET, POST, PUT, DELETE, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type, Authorization")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        self.send_header("Content-Length", "0")
        self.end_headers()

    def do_HEAD(self):
        self._dispatch("GET")

    def do_GET(self):
        self._dispatch("GET")

    def do_POST(self):
        self._dispatch("POST")

    def do_PUT(self):
        self._dispatch("PUT")

    def do_DELETE(self):
        self._dispatch("DELETE")

    # ---------- 分发 ----------
    def _dispatch(self, method):
        parsed = urlparse(self.path)
        path = parsed.path or "/"
        try:
            if not path.startswith("/api/"):
                return self._static(path)

            name, params, scope = _match(method, path)
            if not name:
                raise HttpError(404, "接口不存在", "API_NOT_FOUND")

            ctx = Ctx(self)
            ctx.params = params
            ctx.query = {k: v[0] for k, v in parse_qs(parsed.query).items()}

            if scope == AUTH:
                header = self.headers.get("Authorization") or ""
                token = header[7:] if header.startswith("Bearer ") else ""
                payload = security.verify_token(token)
                if not payload or payload.get("role") != "admin":
                    raise HttpError(401, "登录状态失效了，重新登录一下", "UNAUTHORIZED")
                ctx.user = payload

            if method in ("POST", "PUT", "PATCH"):
                ctx.body = self._read_json(MAX_UPLOAD if path == "/api/admin/upload" else 512 * 1024)

            getattr(api, name)(ctx)

        except HttpError as err:
            fail(self, err.status, err.message, err.code)
        except (BrokenPipeError, ConnectionResetError):
            return
        except Exception as err:  # noqa: BLE001 兜底，避免单个请求拖垮进程
            import traceback
            traceback.print_exc()
            try:
                fail(self, 500, "服务端出错了", "INTERNAL_ERROR")
            except Exception:
                pass

    def _read_json(self, limit):
        length = int(self.headers.get("Content-Length") or 0)
        if length <= 0:
            return {}
        if length > limit:
            raise HttpError(413, "请求内容太大", "PAYLOAD_TOO_LARGE")
        raw = self.rfile.read(length)
        try:
            data = json.loads(raw.decode("utf-8") or "{}")
        except (json.JSONDecodeError, UnicodeDecodeError):
            raise HttpError(400, "请求体不是合法 JSON", "BAD_JSON")
        return data if isinstance(data, dict) else {}

    # ---------- 静态文件 ----------
    def _static(self, path):
        if path == "/":
            return self._send_file(WEB_ROOT / "index.html")

        if path.startswith("/uploads/"):
            target = store.UPLOAD_DIR / posixpath.basename(path)
            return self._send_file(target, public=True)

        rel = path.lstrip("/")
        if rel in ("admin", "admin/"):
            rel = "admin/index.html"
        candidate = (WEB_ROOT / rel).resolve()
        try:
            candidate.relative_to(WEB_ROOT)
        except ValueError:
            return self._not_found()

        if candidate.is_dir():
            candidate = candidate / "index.html"
        if not candidate.exists() and not candidate.suffix:
            candidate = candidate.with_suffix(".html")
        return self._send_file(candidate)

    def _send_file(self, target, public=False):
        target = Path(target)
        if not target.is_file():
            return self._not_found()
        ext = target.suffix.lower()
        ctype = mimetypes.guess_type(str(target))[0] or "application/octet-stream"
        if ext in (".html", ".css", ".js", ".json", ".svg", ".txt"):
            ctype += "; charset=utf-8" if ext != ".svg" else ""
        size = target.stat().st_size
        self.send_response(200)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(size))
        self.send_header("Cache-Control", "public, max-age=3600" if public or ext != ".html" else "no-cache")
        self.send_header("X-Content-Type-Options", "nosniff")
        if ext == ".html":
            self.send_header(
                "Content-Security-Policy",
                "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; "
                "script-src 'self'; object-src 'none'; base-uri 'self'",
            )
        if ext == ".svg":
            self.send_header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
        self.end_headers()
        if self.command != "HEAD":
            with open(target, "rb") as handle:
                while True:
                    chunk = handle.read(65536)
                    if not chunk:
                        break
                    self.wfile.write(chunk)

    def _not_found(self):
        html = (
            "<!doctype html><meta charset=\"utf-8\"><title>404 · 无定电竞护航</title>"
            "<body style=\"background:#070a12;color:#c7d2e4;font-family:system-ui;display:grid;"
            "place-items:center;height:100vh;margin:0\"><div style=\"text-align:center\">"
            "<h1 style=\"font-size:52px;margin:0;color:#35e6ff\">404</h1>"
            "<p style=\"opacity:.7\">这个页面不存在，可能链接写错了</p>"
            "<p><a href=\"/\" style=\"color:#35e6ff\">回首页</a></p></div>"
        ).encode("utf-8")
        self.send_response(404)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(html)))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(html)


def main():
    store.load()
    store.UPLOAD_DIR.mkdir(parents=True, exist_ok=True)
    server = ThreadingHTTPServer((HOST, PORT), Handler)
    server.daemon_threads = True
    print("---------------------------------------------")
    print(" 无定电竞护航系统（Python 后端）")
    print(" 前台    http://localhost:%d/" % PORT)
    print(" 后台    http://localhost:%d/admin/" % PORT)
    print(" 数据    %s" % store.DB_FILE)
    print(" 适配    Termux / Linux，Ctrl+C 停止")
    print("---------------------------------------------")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\n已停止")
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
