package main

// 打手 / 客服 / 管事工作台接口。

import (
	"strings"
	"time"
)

func staffView(o *Order, mask bool) map[string]any {
	phone := o.ContactPhone
	if mask {
		phone = maskPhone(phone)
	}
	progress := o.Progress
	if progress == nil {
		progress = []Progress{}
	}
	return map[string]any{
		"id": o.ID, "orderNo": o.OrderNo, "productTitle": o.ProductTitle,
		"game": o.Game, "category": o.Category, "qty": o.Qty, "amount": o.Amount,
		"status": o.Status, "payStatus": firstNonEmpty(o.PayStatus, "unpaid"),
		"payMethod": o.PayMethod, "payAt": o.PayAt,
		"contactName": o.ContactName, "contactPhone": phone,
		"gameAccount": o.GameAccount, "server": o.Server, "note": o.Note,
		"supportNote": o.SupportNote, "assigneeId": o.AssigneeID, "assigneeName": o.AssigneeName,
		"assignedAt": o.AssignedAt, "progress": progress,
		"createdAt": o.CreatedAt, "updatedAt": o.UpdatedAt,
	}
}

func handleStaffOrders(c *ctx, _ map[string]any) error {
	db, user, err := currentUser(c)
	if err != nil {
		return err
	}
	scope := firstNonEmpty(c.query.Get("scope"), "mine")
	status := firstNonEmpty(c.query.Get("status"), "all")
	q := strings.ToLower(strings.TrimSpace(c.query.Get("q")))

	items := []Order{}
	for _, o := range db.Orders {
		if user.Role == "booster" {
			if scope == "pool" {
				if o.AssigneeID != "" || o.Status != "pending" || firstNonEmpty(o.PayStatus, "unpaid") != "paid" {
					continue
				}
			} else if o.AssigneeID != user.ID {
				continue
			}
		}
		if status != "all" && o.Status != status {
			continue
		}
		if q != "" {
			hay := strings.ToLower(strings.Join([]string{
				o.OrderNo, o.ProductTitle, o.GameAccount, o.ContactName, o.AssigneeName,
			}, " "))
			if !strings.Contains(hay, q) {
				continue
			}
		}
		items = append(items, o)
	}

	out := []map[string]any{}
	for i := range items {
		out = append(out, staffView(&items[i], user.Role == "booster"))
	}
	okData(c.w, map[string]any{"total": len(out), "items": out, "role": user.Role})
	return nil
}

func findStaffOrder(db *DB, ctx *ctx, id string) (*Order, *APIError) {
	for i := range db.Orders {
		o := &db.Orders[i]
		if o.ID != id && o.OrderNo != id {
			continue
		}
		role, _ := ctx.user["role"].(string)
		uid, _ := ctx.user["uid"].(string)
		if role == "booster" && o.AssigneeID != uid {
			return nil, apiError(403, "这单不是派给你的", "FORBIDDEN")
		}
		return o, nil
	}
	return nil, apiError(404, "订单不存在", "ORDER_NOT_FOUND")
}

func handleStaffOrderDetail(c *ctx, _ map[string]any) error {
	db, user, err := currentUser(c)
	if err != nil {
		return err
	}
	o, problem := findStaffOrder(db, c, c.params["id"])
	if problem != nil {
		return problem
	}
	okData(c.w, staffView(o, user.Role == "booster"))
	return nil
}

func handleStaffClaim(c *ctx, _ map[string]any) error {
	_, user, err := currentUser(c)
	if err != nil {
		return err
	}
	if user.Role != "booster" {
		return apiError(403, "只有打手可以接单池里的单", "FORBIDDEN")
	}
	oid := c.params["id"]
	var saved *Order
	var problem *APIError
	_ = Update(func(db *DB) {
		o := findOrder(db, oid)
		if o == nil {
			// 兼容按 id 查找
			for i := range db.Orders {
				if db.Orders[i].ID == oid {
					o = &db.Orders[i]
					break
				}
			}
		}
		if o == nil {
			problem = apiError(404, "订单不存在", "ORDER_NOT_FOUND")
			return
		}
		if o.AssigneeID != "" && o.AssigneeID != user.ID {
			problem = apiError(400, "这单已经被别人接了", "TAKEN")
			return
		}
		if o.Status != "pending" {
			problem = apiError(400, "这单已经在进行或结束了", "BAD_STATUS")
			return
		}
		if firstNonEmpty(o.PayStatus, "unpaid") != "paid" {
			problem = apiError(400, "客户还没付款，先别接", "NOT_PAID")
			return
		}
		o.AssigneeID = user.ID
		o.AssigneeName = user.Name
		o.AssignedAt = nowISO()
		o.Status = "processing"
		o.UpdatedAt = nowISO()
		addLog(db, user.Name+" 接了订单 "+o.OrderNo)
		saved = o
	})
	if problem != nil {
		return problem
	}
	okData(c.w, staffView(saved, true))
	return nil
}

func handleStaffProgress(c *ctx, body map[string]any) error {
	_, user, err := currentUser(c)
	if err != nil {
		return err
	}
	oid := c.params["id"]
	textValue := bodyString(body, "text", 200)
	if textValue == "" {
		return apiError(400, "写一句进度说明", "VALIDATION")
	}
	image := ""
	if v, ok := body["image"].(string); ok && v != "" {
		url, _, imgErr := saveImageData(v, 3*1024*1024, "progress_")
		if imgErr != nil {
			return imgErr
		}
		image = url
	}
	var saved *Order
	var problem *APIError
	_ = Update(func(db *DB) {
		o := findOrder(db, oid)
		if o == nil {
			for i := range db.Orders {
				if db.Orders[i].ID == oid {
					o = &db.Orders[i]
					break
				}
			}
		}
		if o == nil {
			problem = apiError(404, "订单不存在", "ORDER_NOT_FOUND")
			return
		}
		if user.Role == "booster" && o.AssigneeID != user.ID {
			problem = apiError(403, "这单不是派给你的", "FORBIDDEN")
			return
		}
		o.Progress = append(o.Progress, Progress{
			At: nowISO(), By: user.ID, ByName: user.Name, Text: textValue, Image: image,
		})
		if len(o.Progress) > 30 {
			o.Progress = o.Progress[len(o.Progress)-30:]
		}
		if user.Role == "booster" && o.Status == "pending" {
			o.Status = "processing"
		}
		o.UpdatedAt = nowISO()
		addLog(db, user.Name+" 更新订单 "+o.OrderNo+" 进度")
		saved = o
	})
	if problem != nil {
		return problem
	}
	okCreated(c.w, staffView(saved, user.Role == "booster"))
	return nil
}

func handleStaffStatus(c *ctx, body map[string]any) error {
	_, user, err := currentUser(c)
	if err != nil {
		return err
	}
	status := bodyString(body, "status", 20)
	if status != "processing" && status != "done" {
		return apiError(400, "只能改成进行中或已完成", "VALIDATION")
	}
	oid := c.params["id"]
	var saved *Order
	var problem *APIError
	_ = Update(func(db *DB) {
		o := findOrder(db, oid)
		if o == nil {
			for i := range db.Orders {
				if db.Orders[i].ID == oid {
					o = &db.Orders[i]
					break
				}
			}
		}
		if o == nil {
			problem = apiError(404, "订单不存在", "ORDER_NOT_FOUND")
			return
		}
		if user.Role == "booster" && o.AssigneeID != user.ID {
			problem = apiError(403, "这单不是派给你的", "FORBIDDEN")
			return
		}
		o.Status = status
		o.UpdatedAt = nowISO()
		addLog(db, user.Name+" 把订单 "+o.OrderNo+" 标记为 "+status)
		saved = o
	})
	if problem != nil {
		return problem
	}
	okData(c.w, staffView(saved, user.Role == "booster"))
	return nil
}

func handleStaffSummary(c *ctx, _ map[string]any) error {
	db, user, err := currentUser(c)
	if err != nil {
		return err
	}
	month := time.Now().Format("2006-01")
	if user.Role == "booster" {
		assigned, processing, done := 0, 0, 0
		monthDone := 0
		amount := 0.0
		for _, o := range db.Orders {
			if o.AssigneeID != user.ID {
				continue
			}
			assigned++
			switch o.Status {
			case "processing":
				processing++
			case "done":
				done++
				if strings.HasPrefix(o.UpdatedAt, month) {
					monthDone++
					amount += o.Amount
				}
			}
		}
		pool := 0
		for _, o := range db.Orders {
			if o.AssigneeID == "" && o.Status == "pending" && firstNonEmpty(o.PayStatus, "unpaid") == "paid" {
				pool++
			}
		}
		okData(c.w, map[string]any{
			"role": user.Role, "assigned": assigned, "processing": processing, "done": done,
			"monthDone": monthDone, "monthAmount": round2(amount), "share": user.Share,
			"estimate": round2(amount * float64(user.Share) / 100.0), "pool": pool,
		})
		return nil
	}

	today := time.Now().Format("2006-01-02")
	counts := map[string]int{"pending": 0, "processing": 0, "doneToday": 0,
		"unpaid": 0, "review": 0, "unassigned": 0, "boosters": 0}
	for _, o := range db.Orders {
		switch o.Status {
		case "pending":
			counts["pending"]++
		case "processing":
			counts["processing"]++
		case "done":
			if strings.HasPrefix(o.UpdatedAt, today) {
				counts["doneToday"]++
			}
		}
		switch firstNonEmpty(o.PayStatus, "unpaid") {
		case "unpaid":
			counts["unpaid"]++
		case "review":
			counts["review"]++
		}
		if o.AssigneeID == "" && o.Status != "cancelled" {
			counts["unassigned"]++
		}
	}
	for _, u := range db.Users {
		if u.Role == "booster" && u.Status != "off" {
			counts["boosters"]++
		}
	}
	okData(c.w, map[string]any{
		"role": user.Role, "total": len(db.Orders), "pending": counts["pending"],
		"processing": counts["processing"], "doneToday": counts["doneToday"],
		"unpaid": counts["unpaid"], "review": counts["review"],
		"unassigned": counts["unassigned"], "boosters": counts["boosters"],
	})
	return nil
}
