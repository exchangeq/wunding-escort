# 服务器部署

## Python

```bash
unzip wunding-escort-python-2.2.0.zip
cd wunding-escort-python-2.2.0

sudo cp deploy/wunding-escort-py.service /etc/systemd/system/
sudo mkdir -p /opt/wunding-escort
sudo cp -R . /opt/wunding-escort/
sudo chown -R www-data:www-data /opt/wunding-escort
sudo systemctl daemon-reload
sudo systemctl enable --now wunding-escort-py
```

systemd 文件里记得把 `SESSION_SECRET` 换成随机值，`WorkingDirectory` 按实际路径改。

四个入口都在同一个端口：`/` 前台、`/admin/` 董事长、`/manager/` 管事、`/support/` 客服、`/staff/` 打手。Nginx 反代整个站点即可，不需要为每个端单独配域名。

## Go

```bash
unzip wunding-escort-go-2.2.0-linux-amd64.zip
cd wunding-escort-go-2.2.0-linux-amd64

sudo cp ../deploy/wunding-escort-go.service /etc/systemd/system/
sudo mkdir -p /opt/wunding-escort
sudo cp -R . /opt/wunding-escort/
sudo systemctl enable --now wunding-escort-go
```

## Nginx

```bash
sudo cp deploy/nginx.conf.example /etc/nginx/conf.d/wunding-escort.conf
sudo nginx -t && sudo systemctl reload nginx
sudo certbot --nginx -d escort.example.com
```

## 备份与恢复

只有两样东西值钱：`data/db.json` 和 `data/uploads/`。

```bash
tar czf /backup/wunding-$(date +%F).tar.gz data/
```

恢复：停服务 → 覆盖 data 目录 → 起服务。

## 换后端

Python 和 Go 用同一份 `data/db.json`，管理员密码哈希（PBKDF2）和令牌格式也一样。直接把 data 目录拷到新目录，换二进制启动即可，不用重新建账号。

## 常见问题

- **8080 被占用**：`PORT=8090` 再启动。
- **上传 413**：Nginx 加 `client_max_body_size 8m;`。
- **忘了管理员密码**：停服务，编辑 `data/db.json` 把 `admin.passwordHash` 删掉，重启会按 `ADMIN_PASS` 重新生成（订单数据不动）。
- **订单不显示**：先看 `/api/site` 是否正常，再检查 `DATA_DIR` 有没有指错。
