package main

// 支付：收款码人工核对 + 易支付（彩虹易支付兼容）标准接口。
// 签名规则与 Python 版完全一致，换后端不影响已配置的商户参数。

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var payActions = map[string]string{"confirm": "paid", "refund": "refunded", "unpaid": "unpaid"}

// publicPayment 去掉商户密钥，只把前台需要的字段发出去。
func publicPayment(p Payment) Payment {
	out := Payment{
		Manual: PayManual{
			Enabled:  p.Manual.Enabled,
			WechatQr: p.Manual.WechatQr,
			AlipayQr: p.Manual.AlipayQr,
			Note:     p.Manual.Note,
		},
		Epay: PayEpay{
			Enabled: p.Epay.Enabled && p.Epay.Gateway != "" && p.Epay.Pid != "" && p.Epay.Key != "",
			Name:    p.Epay.Name,
			Type:    p.Epay.Type,
		},
	}
	if out.Epay.Name == "" {
		out.Epay.Name = "在线支付"
	}
	if out.Epay.Type == "" {
		out.Epay.Type = "alipay"
	}
	return out
}

func baseURL(r *http.Request) string {
	proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := r.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return proto + "://" + host
}

func epaySign(values map[string]string, key string) string {
	keys := make([]string, 0, len(values))
	for k, v := range values {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
			// 空值不参与签名
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+values[k])
	}
	sum := md5.Sum([]byte(strings.Join(parts, "&") + key))
	return hex.EncodeToString(sum[:])
}

func formValues(r *http.Request) map[string]string {
	_ = r.ParseForm()
	out := map[string]string{}
	for k, v := range r.Form {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

func findOrder(db *DB, no string) *Order {
	no = strings.ToUpper(strings.TrimSpace(no))
	for i := range db.Orders {
		if strings.ToUpper(db.Orders[i].OrderNo) == no {
			return &db.Orders[i]
		}
	}
	return nil
}

// saveImageData 校验并落盘一张 base64 图片，返回访问路径。
func saveImageData(dataURL string, limit int, prefix string) (string, int, error) {
	if !strings.HasPrefix(dataURL, "data:") || !strings.Contains(dataURL, ",") {
		return "", 0, apiError(400, "图片数据不对", "BAD_IMAGE")
	}
	header, payload, _ := strings.Cut(dataURL[5:], ",")
	mime := strings.ToLower(strings.SplitN(header, ";", 2)[0])
	meta, ok := imageMagics[mime]
	if !ok {
		return "", 0, apiError(400, "只支持 PNG / JPG / GIF / WEBP", "UNSUPPORTED_IMAGE")
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", 0, apiError(400, "图片数据损坏", "BAD_IMAGE")
	}
	if len(raw) > limit {
		return "", 0, apiError(413, "图片太大", "TOO_LARGE")
	}
	if len(raw) < 12 || !strings.HasPrefix(string(raw), string(meta.magic)) {
		return "", 0, apiError(400, "图片内容和格式不匹配", "BAD_IMAGE")
	}
	sum := sha256.Sum256(raw)
	name := fmt.Sprintf("%s%s_%s.%s", prefix, hex.EncodeToString(sum[:])[:16], randomHex(4), meta.ext)
	if err := os.WriteFile(filepath.Join(uploadDir, name), raw, 0o644); err != nil {
		return "", 0, apiError(500, "图片写入失败", "UPLOAD_ERROR")
	}
	return "/uploads/" + name, len(raw), nil
}

func handleSubmitProof(c *ctx, body map[string]any) error {
	no := strings.ToUpper(strings.TrimSpace(c.params["orderNo"]))
	data, _ := body["data"].(string)
	url, size, err := saveImageData(data, 3*1024*1024, "proof_")
	if err != nil {
		return err
	}
	var saved *Order
	err = Update(func(db *DB) {
		order := findOrder(db, no)
		if order == nil {
			return
		}
		if order.PayStatus == "paid" {
			return
		}
		order.PayStatus = "review"
		if order.PayMethod == "" {
			order.PayMethod = "manual"
		}
		order.PayProof = url
		order.UpdatedAt = nowISO()
		addLog(db, "上传付款凭证 "+order.OrderNo)
		saved = order
	})
	if err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}
	if saved == nil {
		// 订单不存在或已付款，重新判断给出具体原因
		var status string
		var exists bool
		_ = Read(func(db *DB) {
			if o := findOrder(db, no); o != nil {
				exists = true
				status = o.PayStatus
			}
		})
		if !exists {
			return apiError(404, "没查到这个订单号", "ORDER_NOT_FOUND")
		}
		if status == "paid" {
			return apiError(400, "这单已经确认到账了", "ALREADY_PAID")
		}
		return apiError(400, "凭证没记上，重试一次", "RETRY")
	}
	okCreated(c.w, map[string]any{"orderNo": saved.OrderNo, "payStatus": "review", "payProof": url, "size": size})
	return nil
}

func handlePayCreate(c *ctx, _ map[string]any) error {
	no := strings.ToUpper(strings.TrimSpace(c.params["orderNo"]))
	var order *Order
	var payment Payment
	if err := Read(func(db *DB) {
		order = findOrder(db, no)
		payment = db.Settings.Payment
	}); err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}
	if order == nil {
		return apiError(404, "没查到这个订单号", "ORDER_NOT_FOUND")
	}
	if order.PayStatus == "paid" {
		return apiError(400, "这单已经付过了", "ALREADY_PAID")
	}
	epay := payment.Epay
	if !(epay.Enabled && epay.Gateway != "" && epay.Pid != "" && epay.Key != "") {
		return apiError(400, "在线支付还没开通，用收款码付款", "PAY_DISABLED")
	}

	values := map[string]string{
		"pid":          epay.Pid,
		"type":         firstNonEmpty(epay.Type, "alipay"),
		"out_trade_no": order.OrderNo,
		"notify_url":   baseURL(c.r) + "/api/pay/notify",
		"return_url":   baseURL(c.r) + "/api/pay/return",
		"name":         substr(order.ProductTitle, 60),
		"money":        strconv.FormatFloat(round2(order.Amount), 'f', 2, 64),
	}
	values["sign"] = epaySign(values, epay.Key)
	values["sign_type"] = "MD5"

	q := url.Values{}
	for k, v := range values {
		q.Set(k, v)
	}
	gateway := strings.TrimRight(epay.Gateway, "/")
	if !strings.Contains(gateway, "submit.php") {
		gateway += "/submit.php"
	}
	okData(c.w, map[string]any{"orderNo": order.OrderNo, "payUrl": gateway + "?" + q.Encode()})
	return nil
}

func handleEpayNotify(c *ctx, _ map[string]any) error {
	params := map[string]string{}
	for k, v := range c.r.Form {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	for k, v := range c.query {
		if _, ok := params[k]; !ok && len(v) > 0 {
			params[k] = v[0]
		}
	}

	var payment Payment
	if err := Read(func(db *DB) { payment = db.Settings.Payment }); err != nil {
		writePlain(c.w, "fail")
		return nil
	}
	sign := strings.ToLower(params["sign"])
	if payment.Epay.Key == "" || sign == "" || sign != epaySign(params, payment.Epay.Key) {
		writePlain(c.w, "fail")
		return nil
	}
	if status := params["trade_status"]; status != "TRADE_SUCCESS" && status != "TRADE_FINISHED" {
		writePlain(c.w, "fail")
		return nil
	}
	no := strings.ToUpper(strings.TrimSpace(params["out_trade_no"]))
	paid, _ := strconv.ParseFloat(params["money"], 64)
	done := false
	if err := Update(func(db *DB) {
		order := findOrder(db, no)
		if order == nil {
			return
		}
		if diff := paid - order.Amount; diff > 0.011 || diff < -0.011 {
			return
		}
		if order.PayStatus != "paid" {
			order.PayStatus = "paid"
			order.PayMethod = "epay"
			order.PayAt = nowISO()
			order.PayTradeNo = params["trade_no"]
			order.UpdatedAt = nowISO()
			addLog(db, "在线支付到账 "+order.OrderNo)
		}
		done = true
	}); err != nil {
		writePlain(c.w, "fail")
		return nil
	}
	if done {
		writePlain(c.w, "success")
	} else {
		writePlain(c.w, "fail")
	}
	return nil
}

func writePlain(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = io.WriteString(w, text)
}

func handleEpayReturn(c *ctx, _ map[string]any) error {
	no := strings.ToUpper(strings.TrimSpace(c.query.Get("out_trade_no")))
	target := "/track.html"
	if no != "" {
		target += "?no=" + url.QueryEscape(no)
	}
	http.Redirect(c.w, c.r, target, http.StatusFound)
	return nil
}

func handleAdminOrderPayment(c *ctx, body map[string]any) error {
	id := c.params["id"]
	action := bodyString(body, "action", 20)
	status, ok := payActions[action]
	if !ok {
		return apiError(400, "操作不对", "VALIDATION")
	}
	var saved *Order
	if err := Update(func(db *DB) {
		for i := range db.Orders {
			o := &db.Orders[i]
			if o.ID != id && o.OrderNo != id {
				continue
			}
			o.PayStatus = status
			if status == "paid" {
				o.PayAt = nowISO()
				if o.PayMethod == "" {
					o.PayMethod = "manual"
				}
			}
			if status == "unpaid" {
				o.PayAt = ""
				o.PayTradeNo = ""
			}
			o.UpdatedAt = nowISO()
			addLog(db, "订单 "+o.OrderNo+" 付款状态 → "+status)
			saved = o
			return
		}
	}); err != nil {
		return apiError(500, "数据读取失败", "STORE_ERROR")
	}
	if saved == nil {
		return apiError(404, "订单不存在", "ORDER_NOT_FOUND")
	}
	okData(c.w, saved)
	return nil
}
