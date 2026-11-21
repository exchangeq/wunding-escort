#!/usr/bin/env python3
"""本地演示数据：建客服/打手/管事账号并造几笔不同状态的订单。

用法：先启动服务，再执行
    python3 scripts/demo_seed.py http://127.0.0.1:8080
仅在演示或验收环境使用，不要对生产库执行。
"""
import json
import sys
import urllib.error
import urllib.request

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8080").rstrip("/")
PASSWORD = "pass1234"


def call(path, method="GET", body=None, token=None):
    data = json.dumps(body).encode() if body is not None else None
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(BASE + path, method=method, data=data, headers=headers)
    try:
        with urllib.request.urlopen(req) as res:
            return json.loads(res.read())["data"]
    except urllib.error.HTTPError as err:
        detail = json.loads(err.read())
        raise SystemExit("请求失败 %s: %s" % (path, detail.get("error", {}).get("message", err)))


def main():
    token = call("/api/auth/login", "POST", {"username": "wunding", "password": "wunding2026"})["token"]
    people = [
        ("kefu01", "客服小周", "support", 0),
        ("dashou01", "打手阿凯", "booster", 60),
        ("dashou02", "打手小北", "booster", 60),
        ("guan01", "管事老陈", "manager", 0),
    ]
    for username, name, role, share in people:
        try:
            call("/api/admin/users", "POST", {"username": username, "name": name, "role": role,
                                              "password": PASSWORD, "share": share}, token)
        except SystemExit as err:
            if "已经有人用了" not in str(err):
                raise
    ids = {u["username"]: u["id"] for u in call("/api/admin/users", token=token)["items"]}
    products = call("/api/products")["items"]
    names = ["林先生", "陈小姐", "张同学", "周先生", "王女士", "赵先生"]
    orders = []
    for i, name in enumerate(names):
        product = products[i % len(products)]
        orders.append(call("/api/orders", "POST", {
            "productId": product["id"], "contactName": name,
            "contactPhone": "1380000%04d" % (2000 + i),
            "gameAccount": "峡谷%s%d" % (name[0], 300 + i),
            "server": "微信区", "note": "晚上八点后开始",
        }))
    for order in orders[:4]:
        call("/api/admin/orders/%s/payment" % order["orderNo"], "PUT", {"action": "confirm"}, token)
    call("/api/admin/orders/%s/assign" % orders[0]["orderNo"], "PUT", {"userId": ids["dashou01"]}, token)
    call("/api/admin/orders/%s/assign" % orders[1]["orderNo"], "PUT", {"userId": ids["dashou02"]}, token)
    call("/api/admin/orders/%s" % orders[2]["orderNo"], "PUT", {"status": "processing"}, token)
    call("/api/admin/orders/%s" % orders[3]["orderNo"], "PUT", {"status": "done"}, token)
    call("/api/admin/orders/%s" % orders[4]["orderNo"], "PUT", {"supportNote": "客户要求周末打，先别排"}, token)
    call("/api/staff/orders/%s/progress" % orders[0]["orderNo"], "POST",
         {"text": "已上号，打完第一局，目前两胜"}, token)
    print("演示数据已就绪：kefu01 / dashou01 / dashou02 / guan01，密码 %s" % PASSWORD)


if __name__ == "__main__":
    main()
