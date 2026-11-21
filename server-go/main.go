package main

// 用 Go 部署到独立服务器时跑这个：单文件二进制，不依赖运行时。

import (
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type route struct {
	method  string
	pattern string
	scope   string
	handler func(*ctx, map[string]any) error
}

var allowedRoles = map[string]map[string]bool{
	"staff":   {"owner": true, "manager": true, "support": true, "booster": true},
	"support": {"owner": true, "manager": true, "support": true},
	"manager": {"owner": true, "manager": true},
	"owner":   {"owner": true},
}

var routes = []route{
	{"GET", "/api/health", "public", handleHealth},
	{"GET", "/api/site", "public", handleSite},
	{"GET", "/api/products", "public", handleProducts},
	{"GET", "/api/products/{id}", "public", handleProductDetail},
	{"POST", "/api/orders", "public", handleCreateOrder},
	{"GET", "/api/orders/{orderNo}", "public", handleTrackOrder},
	{"POST", "/api/orders/{orderNo}/proof", "public", handleSubmitProof},
	{"POST", "/api/orders/{orderNo}/pay", "public", handlePayCreate},
	{"GET", "/api/pay/notify", "public", handleEpayNotify},
	{"POST", "/api/pay/notify", "public", handleEpayNotify},
	{"GET", "/api/pay/return", "public", handleEpayReturn},
	{"POST", "/api/admin/login", "public", handleLogin},
	{"POST", "/api/auth/login", "public", handleLogin},
	{"GET", "/api/admin/me", "staff", handleMe},
	{"GET", "/api/auth/me", "staff", handleMe},
	{"POST", "/api/auth/password", "staff", handleChangePassword},
	{"POST", "/api/admin/password", "staff", handleChangePassword},
	{"GET", "/api/staff/orders", "staff", handleStaffOrders},
	{"GET", "/api/staff/orders/{id}", "staff", handleStaffOrderDetail},
	{"POST", "/api/staff/orders/{id}/claim", "staff", handleStaffClaim},
	{"POST", "/api/staff/orders/{id}/progress", "staff", handleStaffProgress},
	{"POST", "/api/staff/orders/{id}/status", "staff", handleStaffStatus},
	{"GET", "/api/staff/summary", "staff", handleStaffSummary},
	{"GET", "/api/admin/users", "manager", handleAdminUsers},
	{"POST", "/api/admin/users", "manager", handleCreateUser},
	{"PUT", "/api/admin/users/{id}", "manager", handleUpdateUser},
	{"DELETE", "/api/admin/users/{id}", "owner", handleDeleteUser},
	{"GET", "/api/admin/logs", "manager", handleAdminLogs},
	{"PUT", "/api/admin/orders/{id}/assign", "manager", handleAssignOrder},
	{"GET", "/api/admin/overview", "support", handleOverview},
	{"GET", "/api/admin/products", "manager", handleAdminProducts},
	{"POST", "/api/admin/products", "manager", handleCreateProduct},
	{"PUT", "/api/admin/products/{id}", "manager", handleUpdateProduct},
	{"DELETE", "/api/admin/products/{id}", "owner", handleDeleteProduct},
	{"GET", "/api/admin/orders", "support", handleAdminOrders},
	{"PUT", "/api/admin/orders/{id}", "support", handleUpdateOrder},
	{"PUT", "/api/admin/orders/{id}/payment", "support", handleAdminOrderPayment},
	{"DELETE", "/api/admin/orders/{id}", "owner", handleDeleteOrder},
	{"GET", "/api/admin/settings", "manager", handleGetSettings},
	{"PUT", "/api/admin/settings", "manager", handleUpdateSettings},
	{"POST", "/api/admin/upload", "staff", handleUpload},
}

var webRoot = func() string {
	root := os.Getenv("WEB_DIR")
	if root == "" {
		root = "web"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	return abs
}()

func matchRoute(method, path string) (*route, map[string]string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		parts = nil
	}
	for i := range routes {
		r := &routes[i]
		if r.method != method {
			continue
		}
		segments := strings.Split(strings.Trim(r.pattern, "/"), "/")
		if len(segments) != len(parts) {
			continue
		}
		params := map[string]string{}
		ok := true
		for j, seg := range segments {
			if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
				params[strings.Trim(seg, "{}")] = parts[j]
				continue
			}
			if seg != parts[j] {
				ok = false
				break
			}
		}
		if ok {
			return r, params
		}
	}
	return nil, nil
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func dispatch(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if !strings.HasPrefix(path, "/api/") {
		serveStatic(w, r)
		return
	}

	matched, params := matchRoute(r.Method, path)
	if matched == nil {
		failMsg(w, 404, "接口不存在", "API_NOT_FOUND")
		return
	}

	c := &ctx{w: w, r: r, params: params, query: r.URL.Query(), ip: clientIP(r)}

	if matched.scope != "public" {
		header := r.Header.Get("Authorization")
		token := strings.TrimPrefix(header, "Bearer ")
		payload := verifyToken(token)
		role, _ := payload["role"].(string)
		if payload == nil || !allowedRoles[matched.scope][role] {
			if payload == nil {
				failMsg(w, 401, "登录状态失效了，重新登录一下", "UNAUTHORIZED")
			} else {
				failMsg(w, 403, "这个账号没有该操作权限", "FORBIDDEN")
			}
			return
		}
		c.user = payload
	}

	if path == "/api/pay/notify" {
		_ = r.ParseForm()
	}

	limit := int64(512 * 1024)
	if path == "/api/admin/upload" || strings.HasSuffix(path, "/proof") {
		limit = 8 * 1024 * 1024
	}
	if path != "/api/pay/notify" {
		if err := c.readBody(limit); err != nil {
			if apiErr, ok := err.(*APIError); ok {
				failMsg(w, apiErr.Status, apiErr.Message, apiErr.Code)
				return
			}
			failMsg(w, 400, "请求格式不对", "BAD_REQUEST")
			return
		}
	}

	if err := matched.handler(c, c.body); err != nil {
		if apiErr, ok := err.(*APIError); ok {
			failMsg(w, apiErr.Status, apiErr.Message, apiErr.Code)
			return
		}
		log.Printf("[error] %s %s: %v", r.Method, path, err)
		failMsg(w, 500, "服务端出错了", "INTERNAL_ERROR")
	}
}

func serveStatic(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		serveLocalFile(w, r, filepath.Join(webRoot, "index.html"))
		return
	}

	if strings.HasPrefix(path, "/uploads/") {
		name := filepath.Base(path)
		serveLocalFile(w, r, filepath.Join(uploadDir, name))
		return
	}

	rel := strings.TrimPrefix(path, "/")
	switch strings.TrimSuffix(rel, "/") {
	case "admin", "manager", "support", "staff":
		rel = strings.TrimSuffix(rel, "/") + "/index.html"
	}
	target := filepath.Join(webRoot, filepath.FromSlash(filepath.Clean(rel)))
	if target != webRoot && !strings.HasPrefix(target, webRoot+string(os.PathSeparator)) {
		serve404(w, r)
		return
	}
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		target = filepath.Join(target, "index.html")
	}
	if _, err := os.Stat(target); os.IsNotExist(err) && filepath.Ext(target) == "" {
		target += ".html"
	}
	serveLocalFile(w, r, target)
}

func serveLocalFile(w http.ResponseWriter, r *http.Request, file string) {
	f, err := os.Open(file)
	if err != nil {
		serve404(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		serve404(w, r)
		return
	}

	ext := strings.ToLower(filepath.Ext(file))
	ctype := mime.TypeByExtension(ext)
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	if ext != ".svg" {
		ctype += "; charset=utf-8"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if ext == ".html" {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; "+
				"script-src 'self'; object-src 'none'; base-uri 'self'")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	if ext == ".svg" {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	}
	http.ServeContent(w, r, filepath.Base(file), info.ModTime(), f)
}

func serve404(w http.ResponseWriter, r *http.Request) {
	const page = `<!doctype html><meta charset="utf-8"><title>404 · 无定电竞护航</title>
<body style="background:#070a12;color:#c7d2e4;font-family:system-ui;display:grid;place-items:center;height:100vh;margin:0">
<div style="text-align:center"><h1 style="font-size:52px;margin:0;color:#35e6ff">404</h1>
<p style="opacity:.7">这个页面不存在，可能链接写错了</p>
<p><a href="/" style="color:#35e6ff">回首页</a></p></div>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(404)
	if r.Method != http.MethodHead {
		w.Write([]byte(page))
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	storeOnce.Do(initStore)
	if storeErr != nil {
		log.Fatalf("数据初始化失败：%v", storeErr)
	}

	addr := net.JoinHostPort(host, port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           http.HandlerFunc(dispatch),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("无定电竞护航系统（Go 后端）监听 %s", addr)
	log.Printf("数据文件 %s", dbFile)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("启动失败：%v", err)
	}
}
