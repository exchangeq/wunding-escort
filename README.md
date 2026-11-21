# 无定电竞护航

工作室自己在用的护航下单系统。前台是接单页，后台改价格、上架项目、看订单，不用装数据库，一个 JSON 文件就够了。

- 前台：项目列表、筛选、详情、下单、付款（收款码 / 在线支付）、订单进度查询
- 董事长端：数据总览、商品、订单、全部账号与权限、站点与支付设置、操作日志
- 管事端：派单、改价上下架、建客服/打手账号、站点文案、日志
- 客服端：订单接待、确认到账、跟进进度、内部备注
- 打手端：接单池、我的任务、提交进度和配图、开始/完成
- 两套后端：Python（Termux / 小机器）和 Go（独立服务器），接口一致，数据文件通用
- 版权：© 无定科技

## 界面

| 前台 | 项目详情与下单 |
| --- | --- |
| ![](preview/01-前台.png) | ![](preview/02-详情.png) |

| 后台总览 | 商品管理 |
| --- | --- |
| ![](preview/03-总览.png) | ![](preview/04-商品.png) |

| 订单付款面板 | 后台收款设置 |
| --- | --- |
| ![](preview/05-付款.png) | ![](preview/06-收款设置.png) |

| 客服工作台 | 管事端派单 |
| --- | --- |
| ![](preview/roles/01.png) | ![](preview/roles/05-manager-orders.png) |

| 打手端（手机） | 董事长端团队账号 |
| --- | --- |
| ![](preview/roles/04-mobile.png) | ![](preview/roles/07-owner-team.png) |

手机端：顶栏改成两行、导航横滑，订单表在手机上变成卡片，不用左右拖着看。

| 前台首页 | 管事端派单 |
| --- | --- |
| ![](preview/mobile/01-首页.png) | ![](preview/mobile/02-管事派单.png) |

| 客服订单队列 |
| --- |
| ![](preview/mobile/03-客服队列.png) |

## 两种部署方式

| | Python 版 | Go 版 |
| --- | --- | --- |
| 适用 | Termux、树莓派、1 核小机器 | 独立服务器 |
| 依赖 | 系统自带 python3，不装 pip 包 | 无，单文件二进制 |
| 启动 | `python3 server-py/app.py` | `./wunding-escort` |
| 并发 | 一般，够小店用 | 好一些 |

两边读写同一个 `data/db.json`，字段、密码哈希、登录令牌都一致，换后端不用重新录数据。四个角色的账号也存在这个文件里，改后端不影响登录。

## 目录

```
wunding-escort/
├── web/                前台和后台的页面，纯静态，不用编译
├── server-py/          Python 后端（标准库）
│   ├── app.py          路由、静态文件、鉴权
│   ├── api.py          接口
│   ├── store.py        数据读写
│   ├── security.py     密码哈希、令牌
│   └── util.py
├── server-go/          Go 后端（标准库）
│   ├── main.go         路由和静态文件
│   ├── handlers.go     接口
│   ├── store.go        数据读写
│   ├── security.go
│   └── types.go
├── termux/             Termux 安装、前台跑、后台跑、停止
├── seed.json           第一次启动用的默认项目和文案，两端共用
├── start.sh            普通 Linux 启动（Python）
├── start-go.sh         普通 Linux 启动（Go）
├── scripts/            冒烟测试、打包脚本
└── deploy/             systemd / nginx 示例
```

## 手机上跑（Termux）

```bash
# 1. 装 Termux（F-Droid 或 GitHub 版本，应用商店那个太旧）
pkg update && pkg install -y git python

# 2. 拉代码
git clone https://github.com/<你的账号>/wunding-escort.git
cd wunding-escort

# 3. 前台跑，Ctrl+C 停
bash termux/run.sh

# 或者放后台跑
bash termux/bg.sh
bash termux/stop.sh
```

打开 `http://127.0.0.1:8080/`。同一个 WiFi 下，电脑用 `http://手机IP:8080/` 也能访问，手机 IP 在「设置 - 关于手机 - 状态信息」里看。

Termux 几个要注意的地方：

1. 装完执行一次 `termux-wake-lock`，不然锁屏后进程容易被系统杀掉（脚本里已经带了）。
2. 后台跑的时候别在最近任务里划掉 Termux，可以在系统设置里给 Termux 关掉电池优化。
3. 想在公网访问，最省事的是用 Cloudflare Tunnel（`pkg install cloudflared`，然后 `cloudflared tunnel --url http://127.0.0.1:8080`），不用折腾端口映射。
4. 数据在 `data/db.json`，图片在 `data/uploads/`，备份直接拷这个目录。

## 独立服务器（Go）

```bash
# 本机有 Go 的话
bash start-go.sh

# 或者直接下 Release 里的 linux-amd64 / arm64 包
unzip wunding-escort-go-2.2.0-linux-amd64.zip
cd wunding-escort-go-2.2.0-linux-amd64
SESSION_SECRET='换成一串随机字符' ./run.sh
```

systemd 示例在 `deploy/`，Nginx 反代也在里面。

## 独立服务器（Python）

```bash
unzip wunding-escort-python-2.2.0.zip
cd wunding-escort-python-2.2.0
SESSION_SECRET='换成一串随机字符' PORT=8080 ./run.sh
```

## 四个入口

| 入口 | 谁用 | 地址 |
| --- | --- | --- |
| 前台 | 客户 | `/` |
| 董事长端 | 老板 / 站长 | `/admin/` |
| 管事端 | 排单、商品、人员 | `/manager/` |
| 客服端 | 接待、确认到账 | `/support/` |
| 打手端 | 接单、报进度 | `/staff/` |

默认只有董事长账号 `wunding` / `wunding2026`，第一次进去先改密码（「账号安全」），再到「团队账号」建客服、打手、管事。管事能建客服和打手，但建不了管事；支付密钥只有董事长能看能改。

分工、权限表和订单流转见 [docs/ROLES.md](docs/ROLES.md)。

环境变量：

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `PORT` | 8080 | 监听端口 |
| `HOST` | 0.0.0.0 | 监听地址，只给本机用就填 127.0.0.1 |
| `DATA_DIR` | data | 数据和上传图片放这里 |
| `ADMIN_USER` / `ADMIN_PASS` | wunding / wunding2026 | 只在第一次初始化时生效 |
| `SESSION_SECRET` | 内置默认值 | 线上一定要改，登录令牌用它签名 |

## 接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/site` | 站点信息、游戏和分类 |
| GET | `/api/products` | 项目列表，支持 game / category / q / sort |
| GET | `/api/products/{id}` | 项目详情 |
| POST | `/api/orders` | 下单，金额由服务端算 |
| GET | `/api/orders/{orderNo}` | 查订单，联系方式脱敏，带付款状态 |
| POST | `/api/orders/{orderNo}/proof` | 上传付款截图 |
| POST | `/api/orders/{orderNo}/pay` | 生成在线支付链接 |
| GET/POST | `/api/pay/notify` | 易支付异步通知，验签后记账 |
| GET | `/api/pay/return` | 支付完成跳回订单页 |
| POST | `/api/auth/login` | 四端统一登录，返回 12 小时令牌与角色 |
| GET | `/api/auth/me` | 当前登录人 |
| POST | `/api/auth/password` | 改自己的密码 |
| GET | `/api/admin/overview` | 总览数据 |
| GET/POST | `/api/admin/products` | 商品列表 / 新增（管事可写，删除仅董事长） |
| PUT/DELETE | `/api/admin/products/{id}` | 改 / 删 |
| GET | `/api/admin/orders` | 订单列表（董事长、管事、客服） |
| PUT/DELETE | `/api/admin/orders/{id}` | 改状态 / 删（删除仅董事长） |
| PUT | `/api/admin/orders/{id}/payment` | 确认到账 / 退款 |
| PUT | `/api/admin/orders/{id}/assign` | 派单 / 取消派单 |
| GET/POST | `/api/admin/users` | 账号列表 / 新建（管事只能看和建客服、打手） |
| PUT/DELETE | `/api/admin/users/{id}` | 改资料、停用、重置密码 / 删除 |
| GET | `/api/admin/logs` | 操作日志 |
| GET/PUT | `/api/admin/settings` | 站点设置（支付字段仅董事长能写） |
| POST | `/api/admin/upload` | 上传图片（base64） |
| GET | `/api/staff/orders` | 打手任务，`scope=pool` 接单池 / `scope=mine` 我的 |
| GET | `/api/staff/orders/{id}` | 打手任务详情（手机号脱敏） |
| POST | `/api/staff/orders/{id}/claim` | 接单 |
| POST | `/api/staff/orders/{id}/progress` | 提交进度，可带 base64 图片 |
| POST | `/api/staff/orders/{id}/status` | 开始 / 完成 |
| GET | `/api/staff/summary` | 打手汇总或客服队列统计 |

所有后台和工作台接口都带 `Authorization: Bearer <token>`；令牌里带角色，越权会直接 403。

## 测试和打包

```bash
bash scripts/smoke.sh python     # 起服务跑下单、支付、四角色权限与派单/进度
bash scripts/smoke.sh go
bash scripts/package-py.sh       # dist/wunding-escort-python-*.zip
bash scripts/package-go.sh       # dist/wunding-escort-go-*-linux-*.zip
```

GitHub Actions（`.github/workflows/release.yml`）会分别测两套后端，然后打包上传；推 `v*` tag 时自动发 Release。产物在 Actions 页面或 Release 里都能下。

更细的部署步骤：Termux 看 [docs/TERMUX.md](docs/TERMUX.md)，服务器和 Nginx 看 [docs/DEPLOY.md](docs/DEPLOY.md)，收款和在线支付看 [docs/PAYMENT.md](docs/PAYMENT.md)，四端分工看 [docs/ROLES.md](docs/ROLES.md)。

演示数据：先启动服务，再执行 `python3 scripts/demo_seed.py http://127.0.0.1:8080`，会建 `kefu01`（客服）、`dashou01` / `dashou02`（打手）、`guan01`（管事），密码统一 `pass1234`，并造几笔不同状态的订单。**不要对生产库执行。**

## 说明

- 订单金额是后端按商品价格算的，前端改不动。
- 登录失败 10 分钟内超过 6 次会锁一会儿。
- 上传只收 PNG/JPG/WEBP/GIF，校验文件头，封面最大 5MB，付款截图 3MB。
- 在线支付走易支付标准接口，签名 MD5，回调会核对金额和交易状态；商户密钥只存服务端，前台接口拿不到。
- 没开在线支付也能用收款码：用户传截图，后台人工确认到账。
- 静态文件做了路径穿越检查，页面响应带 CSP。
- 手机端已经适配：前台、后台、管事、客服、打手五个入口在 390px 宽度下都是单列或两列布局，订单表自动换成卡片。
- 四个角色的权限在后端按路由校验，前端隐藏按钮只是辅助，直接调接口一样会被拒。
- 打手只看到自己的订单和公共接单池；手机号脱敏，付款截图不下发。
- 打手的分成比例只做展示估算，系统不自动结算、不提现。
- 这套系统按「工作室自用」的规模做的，日单量上百没问题；再往上建议换正式数据库。

## 版权

代码、页面和文案归无定科技所有。公开仓库只是为了方便部署和二次开发，不代表放弃著作权，具体见 [LICENSE](LICENSE)。

© 2026 无定科技
