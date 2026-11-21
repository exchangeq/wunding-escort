package main

// 接口实现，和 Python 版一一对应。

import (
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ctx struct {
	w      http.ResponseWriter
	r      *http.Request
	params map[string]string
	query  url.Values
	body   map[string]any
	ip     string
	user   map[string]any
}

var orderStatuses = []string{"pending", "processing", "done", "cancelled"}

func randInt(max int64) int {
	n, err := crand.Int(crand.Reader, big.NewInt(max))
	if err != nil {
		return int(time.Now().UnixNano() % max)
	}
	return int(n.Int64())
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, `{"ok":false,"error":{"message":"序列化失败"}}`, 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	w.Write(raw)
}

func okData(w http.ResponseWriter, data any) {
	writeJSON(w, 200, map[string]any{"ok": true, "data": data})
}

func okCreated(w http.ResponseWriter, data any) {
	writeJSON(w, 201, map[string]any{"ok": true, "data": data})
}

func failMsg(w http.ResponseWriter, status int, message, code string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": map[string]any{"message": message, "code": code}})
}

func (c *ctx) readBody(limit int64) error {
	if c.r.Method != http.MethodPost && c.r.Method != http.MethodPut && c.r.Method != http.MethodPatch {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(c.r.Body, limit))
	if err != nil {
		return apiError(400, "请求读取失败", "BAD_BODY")
	}
	if len(body) > 0 {
		var data map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			return apiError(400, "请求体不是合法 JSON", "BAD_JSON")
		}
		c.body = data
	}
	return nil
}

// ------------------------------------------------ 小工具

func bodyString(m map[string]any, key string, limit int) string {
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	v = strings.TrimSpace(v)
	if limit > 0 && len([]rune(v)) > limit {
		v = string([]rune(v)[:limit])
	}
	return v
}

func bodyNum(m map[string]any, key string, fallback float64) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		var f float64
		if _, err := fmt.Sscanf(v, "%f", &f); err == nil {
			return f
		}
	}
	return fallback
}

func bodyList(m map[string]any, key string, max int, maxLen int) []string {
	raw, ok := m[key].([]any)
	if !ok {
		return []string{}
	}
	out := []string{}
	for _, item := range raw {
		s, ok := item.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
		if len(out) >= max {
			break
		}
	}
	return out
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

func substr(v string, n int) string {
	runes := []rune(v)
	if len(runes) <= n {
		return v
	}
	return string(runes[:n])
}

func maskPhone(v string) string {
	if len(v) < 7 {
		if v == "" {
			return ""
		}
		return "***"
	}
	return v[:3] + "****" + v[len(v)-4:]
}

// ------------------------------------------------ 前台

func handleHealth(c *ctx, _ map[string]any) error {
	okData(c.w, map[string]any{"status": "ok", "time": nowISO()})
	return nil
}

func handleSite(c *ctx, _ map[string]any) error {
	var (
		settings Settings
		products []Product
		meta     Meta
	)
	if err := Read(func(db *DB) {
		settings = db.Settings
		meta = db.Meta
		for _, p := range db.Products {
			if p.Status != "off" {
				products = append(products, p)
			}
		}
	}); err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}

	settings.Payment = publicPayment(settings.Payment)

	games := []map[string]any{}
	counts := map[string]int{}
	order := []string{}
	for _, p := range products {
		if counts[p.Game] == 0 {
			order = append(order, p.Game)
		}
		counts[p.Game]++
	}
	for _, name := range order {
		games = append(games, map[string]any{"name": name, "count": counts[name]})
	}
	categories := []string{}
	seen := map[string]bool{}
	for _, p := range products {
		if !seen[p.Category] {
			seen[p.Category] = true
			categories = append(categories, p.Category)
		}
	}

	okData(c.w, map[string]any{
		"settings":     settings,
		"games":        games,
		"categories":   categories,
		"productCount": len(products),
		"updatedAt":    firstNonEmpty(meta.UpdatedAt, meta.CreatedAt),
	})
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func handleProducts(c *ctx, _ map[string]any) error {
	q := strings.ToLower(strings.TrimSpace(c.query.Get("q")))
	game := strings.TrimSpace(c.query.Get("game"))
	category := strings.TrimSpace(c.query.Get("category"))
	sortBy := c.query.Get("sort")

	items := []Product{}
	if err := Read(func(db *DB) {
		for _, p := range db.Products {
			if p.Status == "off" {
				continue
			}
			if game != "" && game != "全部" && p.Game != game {
				continue
			}
			if category != "" && category != "全部" && p.Category != category {
				continue
			}
			if q != "" {
				hay := strings.ToLower(p.Title + " " + p.Subtitle + " " + p.Game + " " + p.Category + " " + strings.Join(p.Tags, " "))
				if !strings.Contains(hay, q) {
					continue
				}
			}
			items = append(items, p)
		}
	}); err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}

	switch sortBy {
	case "sales":
		sortSlice(items, func(a, b Product) bool { return a.Sales > b.Sales })
	case "priceAsc":
		sortSlice(items, func(a, b Product) bool { return a.Price < b.Price })
	case "priceDesc":
		sortSlice(items, func(a, b Product) bool { return a.Price > b.Price })
	case "new":
		sortSlice(items, func(a, b Product) bool { return a.CreatedAt > b.CreatedAt })
	default:
		sortSlice(items, func(a, b Product) bool {
			if a.Sort != b.Sort {
				return a.Sort < b.Sort
			}
			return a.ID < b.ID
		})
	}

	okData(c.w, map[string]any{"total": len(items), "items": items})
	return nil
}

func sortSlice(items []Product, less func(a, b Product) bool) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && less(items[j], items[j-1]); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

func handleProductDetail(c *ctx, _ map[string]any) error {
	id := atoiSafe(c.params["id"])
	var product Product
	found := false
	if err := Read(func(db *DB) {
		for _, p := range db.Products {
			if p.ID == id && p.Status != "off" {
				product = p
				found = true
				return
			}
		}
	}); err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}
	if !found {
		return apiError(404, "商品不存在或已下架", "PRODUCT_NOT_FOUND")
	}
	okData(c.w, product)
	return nil
}

func atoiSafe(v string) int {
	n := 0
	fmt.Sscanf(v, "%d", &n)
	return n
}

func handleCreateOrder(c *ctx, body map[string]any) error {
	productID := int(bodyNum(body, "productId", 0))
	qty := int(bodyNum(body, "qty", 1))
	if qty < 1 {
		qty = 1
	}
	if qty > 50 {
		qty = 50
	}
	contactName := bodyString(body, "contactName", 40)
	contactPhone := bodyString(body, "contactPhone", 40)
	gameAccount := bodyString(body, "gameAccount", 120)
	server := bodyString(body, "server", 60)
	note := bodyString(body, "note", 500)

	if contactName == "" {
		return apiError(400, "请填写称呼", "VALIDATION")
	}
	if len([]rune(contactPhone)) < 5 {
		return apiError(400, "请填写手机号 / QQ / 微信，方便值班联系你", "VALIDATION")
	}
	if gameAccount == "" {
		return apiError(400, "请填写游戏账号或角色 ID", "VALIDATION")
	}

	var order Order
	var settings Settings
	notFound := true
	err := Update(func(db *DB) {
		settings = db.Settings
		for i := range db.Products {
			p := &db.Products[i]
			if p.ID != productID || p.Status == "off" {
				continue
			}
			notFound = false
			if qty > p.Stock {
				return
			}
			no := orderNo()
			amount := round2(p.Price * float64(qty))
			order = Order{
				ID: "ord_" + no, OrderNo: no, ProductID: p.ID,
				ProductTitle: p.Title, Game: p.Game, Category: p.Category, Cover: p.Cover,
				Price: p.Price, Unit: p.Unit, Qty: qty, Amount: amount,
				ContactName: contactName, ContactPhone: contactPhone,
				GameAccount: gameAccount, Server: server, Note: note,
				Status: "pending", PayStatus: "unpaid", Progress: []Progress{},
				CreatedAt: nowISO(), UpdatedAt: nowISO(),
			}
			db.Orders = append([]Order{order}, db.Orders...)
			p.Stock -= qty
			if p.Stock < 0 {
				p.Stock = 0
			}
			addLog(db, "新订单 "+no+" "+p.Title)
			return
		}
	})
	if err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}
	if notFound {
		return apiError(404, "商品不存在或已下架", "PRODUCT_NOT_FOUND")
	}
	if order.OrderNo == "" {
		return apiError(400, "库存不够，先少买点或者问下客服", "NO_STOCK")
	}

	okCreated(c.w, map[string]any{
		"orderNo":      order.OrderNo,
		"amount":       order.Amount,
		"status":       order.Status,
		"productTitle": order.ProductTitle,
		"payStatus":    "unpaid",
		"payment":      publicPayment(settings.Payment),
		"kefu":         settings.Kefu,
		"tips":         settings.OrderTips,
	})
	return nil
}

func handleTrackOrder(c *ctx, _ map[string]any) error {
	no := strings.ToUpper(strings.TrimSpace(c.params["orderNo"]))
	var order Order
	var settings Settings
	found := false
	if err := Read(func(db *DB) {
		settings = db.Settings
		for _, o := range db.Orders {
			if strings.EqualFold(o.OrderNo, no) {
				order = o
				found = true
				return
			}
		}
	}); err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}
	if !found {
		return apiError(404, "没查到这个订单号，核对一下再试", "ORDER_NOT_FOUND")
	}
	okData(c.w, map[string]any{
		"orderNo":      order.OrderNo,
		"productTitle": order.ProductTitle,
		"game":         order.Game,
		"cover":        order.Cover,
		"qty":          order.Qty,
		"amount":       order.Amount,
		"status":       order.Status,
		"contactName":  order.ContactName,
		"contactPhone": maskPhone(order.ContactPhone),
		"gameAccount":  order.GameAccount,
		"server":       order.Server,
		"note":         order.Note,
		"payStatus":    firstNonEmpty(order.PayStatus, "unpaid"),
		"payMethod":    order.PayMethod,
		"payAt":        order.PayAt,
		"payProof":     order.PayProof,
		"payment":      publicPayment(settings.Payment),
		"assigneeName": order.AssigneeName,
		"progress": func() []map[string]any {
			out := []map[string]any{}
			items := order.Progress
			if len(items) > 8 {
				items = items[len(items)-8:]
			}
			for _, p := range items {
				out = append(out, map[string]any{"at": p.At, "byName": p.ByName, "text": p.Text})
			}
			return out
		}(),
		"createdAt": order.CreatedAt,
		"updatedAt": order.UpdatedAt,
		"kefu":      settings.Kefu,
	})
	return nil
}

// ------------------------------------------------ 登录

type loginLimit struct {
	mu      sync.Mutex
	records map[string]*loginRecord
}

type loginRecord struct {
	count   int
	resetAt time.Time
}

var loginLimits = loginLimit{records: map[string]*loginRecord{}}

const loginMax = 6

func (l *loginLimit) check(ip string) *APIError {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	rec, ok := l.records[ip]
	if ok && rec.count >= loginMax && now.Before(rec.resetAt) {
		return apiError(429, fmt.Sprintf("登录太频繁，%d 秒后再试", int(rec.resetAt.Sub(now).Seconds())), "TOO_MANY_ATTEMPTS")
	}
	if !ok || now.After(rec.resetAt) {
		l.records[ip] = &loginRecord{count: 0, resetAt: now.Add(10 * time.Minute)}
	}
	return nil
}

func (l *loginLimit) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec, ok := l.records[ip]
	if !ok {
		rec = &loginRecord{resetAt: time.Now().Add(10 * time.Minute)}
		l.records[ip] = rec
	}
	rec.count++
}

func (l *loginLimit) clear(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.records, ip)
}

func findUser(db *DB, username string) *User {
	for i := range db.Users {
		if strings.EqualFold(db.Users[i].Username, username) {
			return &db.Users[i]
		}
	}
	return nil
}

func userByID(db *DB, id string) *User {
	for i := range db.Users {
		if db.Users[i].ID == id {
			return &db.Users[i]
		}
	}
	return nil
}

func currentUser(c *ctx) (*DB, *User, *APIError) {
	var dbCopy DB
	if err := Read(func(db *DB) { dbCopy = *db }); err != nil {
		return nil, nil, apiError(500, "数据读取失败", "STORE_ERROR")
	}
	uid, _ := c.user["uid"].(string)
	user := userByID(&dbCopy, uid)
	if user == nil || user.Status == "off" {
		return nil, nil, apiError(401, "账号已停用或不存在", "UNAUTHORIZED")
	}
	return &dbCopy, user, nil
}

func userJSON(u *User) map[string]any {
	return map[string]any{
		"id": u.ID, "username": u.Username, "name": u.Name, "role": u.Role,
		"phone": u.Phone, "share": u.Share, "status": u.Status,
		"createdAt": u.CreatedAt, "lastLoginAt": u.LoginAt,
	}
}

func handleLogin(c *ctx, body map[string]any) error {
	if err := loginLimits.check(c.ip); err != nil {
		return err
	}
	username := bodyString(body, "username", 40)
	password := bodyString(body, "password", 100)
	if username == "" || password == "" {
		return apiError(400, "账号和密码都要填", "VALIDATION")
	}

	var found User
	var exists bool
	if err := Read(func(db *DB) {
		if u := findUser(db, username); u != nil {
			found = *u
			exists = true
		}
	}); err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}
	if !exists || found.Status == "off" || !verifyPassword(password, found.PasswordHash) {
		loginLimits.fail(c.ip)
		return apiError(401, "账号或密码不对", "BAD_CREDENTIALS")
	}
	loginLimits.clear(c.ip)

	_ = Update(func(db *DB) {
		if u := findUser(db, username); u != nil {
			u.LoginAt = nowISO()
			if u.Role == "owner" {
				db.Admin.LastLoginAt = nowISO()
			}
			addLog(db, firstNonEmpty(u.Name, u.Username)+" 登录")
		}
	})

	token, err := signToken(found)
	if err != nil {
		return apiError(500, "令牌生成失败", "TOKEN_ERROR")
	}
	public := userJSON(&found)
	okData(c.w, map[string]any{
		"token":     token,
		"expiresIn": tokenTTLSeconds,
		"user":      public,
		"role":      found.Role,
		"admin":     map[string]any{"username": found.Username, "displayName": found.Name},
	})
	return nil
}

func handleMe(c *ctx, _ map[string]any) error {
	var s Settings
	var user User
	if err := Read(func(db *DB) {
		s = db.Settings
		uid, _ := c.user["uid"].(string)
		if u := userByID(db, uid); u != nil {
			user = *u
		}
	}); err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}
	if user.ID == "" {
		return apiError(401, "账号不存在", "UNAUTHORIZED")
	}
	okData(c.w, map[string]any{
		"id": user.ID, "username": user.Username, "name": user.Name,
		"displayName": user.Name, "role": user.Role, "share": user.Share,
		"status": user.Status, "siteName": s.SiteName, "company": s.Company,
		"copyright": s.Copyright, "lastLoginAt": user.LoginAt,
	})
	return nil
}

func handleOverview(c *ctx, _ map[string]any) error {
	var (
		orders   []Order
		products []Product
		logs     []Log
		users    []User
	)
	if err := Read(func(db *DB) {
		orders = append([]Order{}, db.Orders...)
		products = append([]Product{}, db.Products...)
		logs = append([]Log{}, db.Logs...)
		users = append([]User{}, db.Users...)
	}); err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}

	today := time.Now().Format("2006-01-02")
	totals := map[string]any{
		"orders": len(orders), "pending": 0, "processing": 0, "done": 0, "cancelled": 0,
		"todayOrders": 0, "revenue": 0.0, "products": len(products), "onSale": 0,
		"unpaid": 0, "payReview": 0, "paid": 0, "unassigned": 0, "boosters": 0,
	}
	revenue := 0.0
	for _, p := range products {
		if p.Status != "off" {
			totals["onSale"] = totals["onSale"].(int) + 1
		}
	}
	for _, o := range orders {
		if _, ok := totals[o.Status]; ok {
			totals[o.Status] = totals[o.Status].(int) + 1
		}
		if o.Status == "done" {
			revenue += o.Amount
		}
		if strings.HasPrefix(o.CreatedAt, today) {
			totals["todayOrders"] = totals["todayOrders"].(int) + 1
		}
		switch firstNonEmpty(o.PayStatus, "unpaid") {
		case "unpaid":
			totals["unpaid"] = totals["unpaid"].(int) + 1
		case "review":
			totals["payReview"] = totals["payReview"].(int) + 1
		case "paid":
			totals["paid"] = totals["paid"].(int) + 1
		}
	}
	for _, u := range users {
		if u.Role == "booster" && u.Status != "off" {
			totals["boosters"] = totals["boosters"].(int) + 1
		}
	}
	for _, o := range orders {
		if o.AssigneeID == "" && o.Status != "cancelled" {
			totals["unassigned"] = totals["unassigned"].(int) + 1
		}
	}
	totals["revenue"] = round2(revenue)

	series := []map[string]any{}
	for i := 6; i >= 0; i-- {
		day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		count := 0
		amount := 0.0
		for _, o := range orders {
			if strings.HasPrefix(o.CreatedAt, day) {
				count++
				amount += o.Amount
			}
		}
		series = append(series, map[string]any{"date": day[5:], "count": count, "amount": round2(amount)})
	}

	recent := orders
	if len(recent) > 8 {
		recent = recent[:8]
	}
	recentLogs := logs
	if len(recentLogs) > 6 {
		recentLogs = recentLogs[:6]
	}

	okData(c.w, map[string]any{
		"totals":       totals,
		"series":       series,
		"recentOrders": recent,
		"logs":         recentLogs,
	})
	return nil
}

// ------------------------------------------------ 商品管理

func handleAdminProducts(c *ctx, _ map[string]any) error {
	q := strings.ToLower(strings.TrimSpace(c.query.Get("q")))
	game := strings.TrimSpace(c.query.Get("game"))
	items := []Product{}
	if err := Read(func(db *DB) {
		for _, p := range db.Products {
			if game != "" && game != "全部" && p.Game != game {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(p.Title+" "+p.Game+" "+p.Category), q) {
				continue
			}
			items = append(items, p)
		}
	}); err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}
	items = sortedProducts(items)
	okData(c.w, map[string]any{"total": len(items), "items": items})
	return nil
}

func applyProduct(old Product, body map[string]any) (Product, *APIError) {
	p := old
	title := bodyString(body, "title", 120)
	if title == "" {
		title = p.Title
	}
	if title == "" {
		return p, apiError(400, "商品标题不能空", "VALIDATION")
	}

	p.Title = title
	p.Game = firstNonEmpty(bodyString(body, "game", 40), p.Game, "其他")
	p.Category = firstNonEmpty(bodyString(body, "category", 40), p.Category, "段位护航")
	p.Subtitle = bodyString(body, "subtitle", 120)
	p.Cover = firstNonEmpty(bodyString(body, "cover", 500), p.Cover, "/assets/img/cover-default.svg")
	p.Price = round2(bodyNum(body, "price", p.Price))
	if p.Price < 0 {
		p.Price = 0
	}
	p.OldPrice = round2(bodyNum(body, "oldPrice", p.OldPrice))
	if p.OldPrice < 0 {
		p.OldPrice = 0
	}
	p.Unit = firstNonEmpty(bodyString(body, "unit", 12), p.Unit, "局")
	p.Tags = bodyList(body, "tags", 6, 16)
	p.Desc = bodyString(body, "desc", 4000)
	p.Includes = bodyList(body, "includes", 8, 40)
	p.Stock = int(bodyNum(body, "stock", float64(p.Stock)))
	if p.Stock < 0 {
		p.Stock = 0
	}
	p.Sales = int(bodyNum(body, "sales", float64(p.Sales)))
	p.Sort = int(bodyNum(body, "sort", float64(p.Sort)))
	if bodyString(body, "status", 10) == "off" {
		p.Status = "off"
	} else {
		p.Status = "on"
	}
	p.UpdatedAt = nowISO()
	return p, nil
}

func handleCreateProduct(c *ctx, body map[string]any) error {
	var created Product
	err := Update(func(db *DB) {
		product, apiErr := applyProduct(Product{}, body)
		if apiErr != nil {
			return
		}
		product.ID = nextProductID(db)
		product.CreatedAt = nowISO()
		product.UpdatedAt = product.CreatedAt
		db.Products = append(db.Products, product)
		addLog(db, "上架商品 "+product.Title)
		created = product
	})
	if err != nil {
		return apiError(500, "数据写入失败", "STORE_ERROR")
	}
	if created.ID == 0 {
		return apiError(400, "商品标题不能空", "VALIDATION")
	}
	okCreated(c.w, created)
	return nil
}

func handleUpdateProduct(c *ctx, body map[string]any) error {
	id := atoiSafe(c.params["id"])
	var updated Product
	found := false
	var validation error
	_ = Update(func(db *DB) {
		for i := range db.Products {
			if db.Products[i].ID != id {
				continue
			}
			product, apiErr := applyProduct(db.Products[i], body)
			if apiErr != nil {
				validation = apiErr
				return
			}
			db.Products[i] = product
			addLog(db, "更新商品 "+product.Title)
			updated = product
			found = true
			return
		}
	})
	if validation != nil {
		if e, ok := validation.(*APIError); ok {
			return e
		}
		return apiError(400, "参数不合法", "VALIDATION")
	}
	if !found {
		return apiError(404, "商品不存在", "PRODUCT_NOT_FOUND")
	}
	okData(c.w, updated)
	return nil
}

func handleDeleteProduct(c *ctx, _ map[string]any) error {
	id := atoiSafe(c.params["id"])
	removed := false
	_ = Update(func(db *DB) {
		for i, p := range db.Products {
			if p.ID != id {
				continue
			}
			db.Products = append(db.Products[:i], db.Products[i+1:]...)
			addLog(db, "删除商品 "+p.Title)
			removed = true
			return
		}
	})
	if !removed {
		return apiError(404, "商品不存在", "PRODUCT_NOT_FOUND")
	}
	okData(c.w, map[string]any{"id": id})
	return nil
}

// ------------------------------------------------ 订单管理

func handleAdminOrders(c *ctx, _ map[string]any) error {
	status := c.query.Get("status")
	if status == "" {
		status = "all"
	}
	q := strings.ToLower(strings.TrimSpace(c.query.Get("q")))
	items := []Order{}
	if err := Read(func(db *DB) {
		for _, o := range db.Orders {
			if status != "all" && o.Status != status {
				continue
			}
			if q != "" {
				hay := strings.ToLower(strings.Join([]string{o.OrderNo, o.ContactName, o.ContactPhone, o.ProductTitle, o.GameAccount}, " "))
				if !strings.Contains(hay, q) {
					continue
				}
			}
			items = append(items, o)
		}
	}); err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}
	okData(c.w, map[string]any{"total": len(items), "items": items})
	return nil
}

func validStatus(status string) bool {
	for _, s := range orderStatuses {
		if s == status {
			return true
		}
	}
	return false
}

func handleUpdateOrder(c *ctx, body map[string]any) error {
	oid := c.params["id"]
	status := bodyString(body, "status", 20)
	if status != "" && !validStatus(status) {
		return apiError(400, "订单状态不对", "VALIDATION")
	}
	note, hasNote := body["supportNote"].(string)
	var updated Order
	found := false
	_ = Update(func(db *DB) {
		for i := range db.Orders {
			o := &db.Orders[i]
			if o.ID == oid || o.OrderNo == oid {
				if status != "" {
					o.Status = status
				}
				if hasNote {
					o.SupportNote = substr(note, 500)
				}
				o.UpdatedAt = nowISO()
				addLog(db, "订单 "+o.OrderNo+" → "+firstNonEmpty(status, "更新备注"))
				updated = *o
				found = true
				return
			}
		}
	})
	if !found {
		return apiError(404, "订单不存在", "ORDER_NOT_FOUND")
	}
	okData(c.w, updated)
	return nil
}

func handleDeleteOrder(c *ctx, _ map[string]any) error {
	oid := c.params["id"]
	removed := false
	_ = Update(func(db *DB) {
		for i, o := range db.Orders {
			if o.ID == oid || o.OrderNo == oid {
				db.Orders = append(db.Orders[:i], db.Orders[i+1:]...)
				addLog(db, "删除订单 "+o.OrderNo)
				removed = true
				return
			}
		}
	})
	if !removed {
		return apiError(404, "订单不存在", "ORDER_NOT_FOUND")
	}
	okData(c.w, map[string]any{"id": oid})
	return nil
}

// ------------------------------------------------ 站点设置

func handleGetSettings(c *ctx, _ map[string]any) error {
	var s Settings
	if err := Read(func(db *DB) { s = db.Settings }); err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}
	if _, u, err := currentUser(c); err == nil && u.Role != "owner" {
		s.Payment.Epay.Key = ""
	}
	okData(c.w, s)
	return nil
}

func handleUpdateSettings(c *ctx, body map[string]any) error {
	var updated Settings
	_ = Update(func(db *DB) {
		s := &db.Settings
		if _, ok := body["siteName"].(string); ok {
			s.SiteName = bodyString(body, "siteName", 40)
		}
		if v, ok := body["company"].(string); ok && v != "" {
			s.Company = bodyString(body, "company", 40)
		}
		if _, ok := body["slogan"].(string); ok {
			s.Slogan = bodyString(body, "slogan", 120)
		}
		if _, ok := body["subSlogan"].(string); ok {
			s.SubSlogan = bodyString(body, "subSlogan", 160)
		}
		if _, ok := body["notice"].(string); ok {
			s.Notice = bodyString(body, "notice", 300)
		}
		if _, ok := body["copyright"].(string); ok {
			s.Copyright = bodyString(body, "copyright", 80)
		}
		if _, ok := body["icp"].(string); ok {
			s.ICP = bodyString(body, "icp", 80)
		}
		if _, ok := body["adminNotice"].(string); ok {
			s.AdminNotice = bodyString(body, "adminNotice", 300)
		}
		if _, ok := body["orderTips"].(string); ok {
			s.OrderTips = bodyString(body, "orderTips", 300)
		}
		if kefu, ok := body["kefu"].(map[string]any); ok {
			s.Kefu = Kefu{
				QQ: bodyString(kefu, "qq", 40), WeChat: bodyString(kefu, "wechat", 40),
				Phone: bodyString(kefu, "phone", 40), Hours: bodyString(kefu, "hours", 60),
			}
		}
		role, _ := c.user["role"].(string)
		if payment, ok := body["payment"].(map[string]any); role == "owner" && ok {
			manual, _ := payment["manual"].(map[string]any)
			epay, _ := payment["epay"].(map[string]any)
			ptype := bodyString(epay, "type", 10)
			if ptype != "wxpay" {
				ptype = "alipay"
			}
			s.Payment = Payment{
				Manual: PayManual{
					Enabled:  manual["enabled"] == true,
					WechatQr: bodyString(manual, "wechatQr", 300),
					AlipayQr: bodyString(manual, "alipayQr", 300),
					Note:     bodyString(manual, "note", 200),
				},
				Epay: PayEpay{
					Enabled: epay["enabled"] == true,
					Gateway: strings.TrimRight(bodyString(epay, "gateway", 200), "/"),
					Pid:     bodyString(epay, "pid", 40),
					Key:     bodyString(epay, "key", 120),
					Type:    ptype,
					Name:    firstNonEmpty(bodyString(epay, "name", 20), "在线支付"),
				},
			}
		}
		if stats, ok := body["heroStats"].([]any); ok {
			hero := []HeroStat{}
			for _, item := range stats {
				m, ok := item.(map[string]any)
				if !ok || len(hero) >= 4 {
					continue
				}
				hero = append(hero, HeroStat{
					Label: bodyString(m, "label", 20), Value: bodyString(m, "value", 12),
					Suffix: bodyString(m, "suffix", 6),
				})
			}
			s.HeroStats = hero
		}
		addLog(db, "更新站点设置")
		updated = *s
	})
	okData(c.w, updated)
	return nil
}

// ------------------------------------------------ 上传

var imageMagics = map[string]struct {
	ext   string
	magic []byte
}{
	"image/png":  {"png", []byte("\x89PNG")},
	"image/jpeg": {"jpg", []byte("\xff\xd8\xff")},
	"image/gif":  {"gif", []byte("GIF")},
	"image/webp": {"webp", []byte("RIFF")},
}

func handleUpload(c *ctx, body map[string]any) error {
	dataURL, _ := body["data"].(string)
	if !strings.HasPrefix(dataURL, "data:") || !strings.Contains(dataURL, ",") {
		return apiError(400, "图片数据不对", "BAD_IMAGE")
	}
	header, payload, _ := strings.Cut(dataURL[5:], ",")
	mime := strings.SplitN(header, ";", 2)[0]
	meta, ok := imageMagics[strings.ToLower(mime)]
	if !ok {
		return apiError(400, "只支持 PNG / JPG / GIF / WEBP", "UNSUPPORTED_IMAGE")
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return apiError(400, "图片数据损坏", "BAD_IMAGE")
	}
	if len(raw) > 5*1024*1024 {
		return apiError(413, "图片不要超过 5MB", "TOO_LARGE")
	}
	if len(raw) < 12 || !strings.HasPrefix(string(raw), string(meta.magic)) {
		return apiError(400, "图片内容和格式不匹配", "BAD_IMAGE")
	}
	sum := sha256.Sum256(raw)
	name := fmt.Sprintf("img_%s_%s.%s", hex.EncodeToString(sum[:])[:16], randomHex(4), meta.ext)
	path := filepath.Join(uploadDir, name)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return apiError(500, "图片写入失败", "UPLOAD_ERROR")
	}
	_ = Update(func(db *DB) { addLog(db, "上传图片 "+name) })
	okCreated(c.w, map[string]any{"url": "/uploads/" + name, "size": len(raw)})
	return nil
}

func handleChangePassword(c *ctx, body map[string]any) error {
	old := bodyString(body, "oldPassword", 100)
	newPass := bodyString(body, "newPassword", 100)
	if len(newPass) < 6 {
		return apiError(400, "新密码至少 6 位", "VALIDATION")
	}
	uid, _ := c.user["uid"].(string)
	var user User
	if err := Read(func(db *DB) {
		if u := userByID(db, uid); u != nil {
			user = *u
		}
	}); err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}
	if user.ID == "" {
		return apiError(401, "账号不存在", "UNAUTHORIZED")
	}
	if !verifyPassword(old, user.PasswordHash) {
		return apiError(401, "原密码不对", "BAD_CREDENTIALS")
	}
	_ = Update(func(db *DB) {
		if u := userByID(db, uid); u != nil {
			u.PasswordHash = hashPassword(newPass)
			if u.Role == "owner" {
				db.Admin.PasswordHash = u.PasswordHash
			}
			addLog(db, firstNonEmpty(u.Name, u.Username)+" 修改密码")
		}
	})
	okData(c.w, map[string]any{"changed": true})
	return nil
}
