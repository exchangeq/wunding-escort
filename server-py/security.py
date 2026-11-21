"""密码哈希与登录令牌。

密码：PBKDF2-HMAC-SHA256 加盐，和 Go 版保持一致，数据文件可以互相接管。
令牌：base64url(payload).base64url(hmac)，12 小时过期。
"""

import base64
import hashlib
import hmac
import json
import os
import secrets
import time

ITERATIONS = 120000
TOKEN_TTL = 12 * 3600

SECRET = (
    os.environ.get("SESSION_SECRET")
    or os.environ.get("AUTH_SECRET")
    or "wunding-escort-local-secret-please-change"
)


def _b64(raw):
    return base64.urlsafe_b64encode(raw).decode().rstrip("=")


def _unb64(value):
    padding = "=" * (-len(value) % 4)
    return base64.urlsafe_b64decode(value + padding)


def hash_password(password):
    salt = secrets.token_hex(16)
    digest = hashlib.pbkdf2_hmac("sha256", str(password).encode(), salt.encode(), ITERATIONS)
    return "pbkdf2_sha256${}${}${}".format(ITERATIONS, salt, digest.hex())


def verify_password(password, stored):
    if not isinstance(stored, str):
        return False
    parts = stored.split("$")
    if len(parts) != 4 or parts[0] != "pbkdf2_sha256":
        return False
    _, iterations, salt, digest = parts
    try:
        calc = hashlib.pbkdf2_hmac(
            "sha256", str(password).encode(), salt.encode(), int(iterations)
        ).hex()
    except (ValueError, TypeError):
        return False
    return hmac.compare_digest(calc, digest)


def sign(payload):
    body = dict(payload)
    body["exp"] = int(time.time()) + TOKEN_TTL
    data = _b64(json.dumps(body, separators=(",", ":")).encode())
    sig = _b64(hmac.new(SECRET.encode(), data.encode(), hashlib.sha256).digest())
    return data + "." + sig


def verify_token(token):
    if not isinstance(token, str) or "." not in token:
        return None
    data, sig = token.split(".", 1)
    expect = _b64(hmac.new(SECRET.encode(), data.encode(), hashlib.sha256).digest())
    if not hmac.compare_digest(sig, expect):
        return None
    try:
        payload = json.loads(_unb64(data).decode())
    except Exception:
        return None
    if not isinstance(payload, dict) or int(payload.get("exp", 0)) < int(time.time()):
        return None
    return payload
