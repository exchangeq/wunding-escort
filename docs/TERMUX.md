# Termux 部署笔记

## 装

用 F-Droid 或 GitHub 上发布的 Termux，应用商店里那个版本太旧，python3 不一定齐。

```bash
pkg update && pkg install -y git python
git clone <仓库地址> wunding-escort
cd wunding-escort
bash termux/run.sh
```

不需要 `pip install`，后端只用了标准库。

## 后台跑

```bash
bash termux/bg.sh     # 启
bash termux/stop.sh   # 停
tail -f data/termux.log
```

`bg.sh` 会写 `data/termux.pid`，重复执行不会起两个进程。

## 锁屏被杀

1. 每次启动脚本会执行 `termux-wake-lock`；
2. 系统设置里给 Termux 关掉电池优化；
3. 不要从最近任务里划掉 Termux；
4. 真要长期挂着，插电、关屏休眠设成「永不」比较稳。

## 局域访问

手机连 WiFi，电脑浏览器开 `http://手机IP:8080/`。手机 IP 看「设置 - 关于手机 - 状态信息 - IP 地址」。

手机自身只监听 `0.0.0.0`，部分系统会用防火墙拦住外部访问，遇到连不上先关掉手机自带的「网络防护」类功能试试。

## 公网访问

不建议直接把 8080 暴露到公网。两个省事的做法：

```bash
# Cloudflare Tunnel，给一个临时域名
pkg install -y cloudflared
cloudflared tunnel --url http://127.0.0.1:8080
```

或者用 frp / nginx 反代到你自己的服务器，反代时把 `client_max_body_size` 调到 8m，不然传封面会被 413。

## 备份

```bash
tar czf wunding-backup-$(date +%F).tar.gz data/
```

换手机或重装 Termux 前跑一次，把包拷出去。
