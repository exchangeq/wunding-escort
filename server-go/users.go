package main

// 账号管理：董事长管全部，管事只管客服和打手。

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var roleNames = map[string]string{"owner": "董事长", "manager": "管事", "support": "客服", "booster": "打手"}
var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,20}$`)

func manageAllowed(actor *User, targetRole, newRole string) *APIError {
	if actor.Role == "owner" {
		return nil
	}
	if targetRole != "support" && targetRole != "booster" {
		return apiError(403, "管事只能管理客服和打手账号", "FORBIDDEN")
	}
	if newRole != "" && newRole != "support" && newRole != "booster" {
		return apiError(403, "管事只能管理客服和打手账号", "FORBIDDEN")
	}
	return nil
}

func handleAdminUsers(c *ctx, _ map[string]any) error {
	db, actor, err := currentUser(c)
	if err != nil {
		return err
	}
	items := []User{}
	for _, u := range db.Users {
		if actor.Role == "manager" && (u.Role != "support" && u.Role != "booster") {
			continue
		}
		items = append(items, u)
	}
	roleRank := map[string]int{"owner": 0, "manager": 1, "support": 2, "booster": 3}
	sort.SliceStable(items, func(i, j int) bool {
		if roleRank[items[i].Role] != roleRank[items[j].Role] {
			return roleRank[items[i].Role] < roleRank[items[j].Role]
		}
		return items[i].CreatedAt < items[j].CreatedAt
	})
	out := []map[string]any{}
	for i := range items {
		out = append(out, userJSON(&items[i]))
	}
	okData(c.w, map[string]any{"total": len(out), "items": out})
	return nil
}

func handleCreateUser(c *ctx, body map[string]any) error {
	_, actor, err := currentUser(c)
	if err != nil {
		return err
	}
	username := bodyString(body, "username", 20)
	if !usernamePattern.MatchString(username) {
		return apiError(400, "账号只能是 3-20 位字母、数字或下划线", "VALIDATION")
	}
	role := bodyString(body, "role", 10)
	if _, ok := roleNames[role]; !ok {
		return apiError(400, "角色不对", "VALIDATION")
	}
	if problem := manageAllowed(actor, role, ""); problem != nil {
		return problem
	}
	if role == "owner" {
		return apiError(400, "董事长账号只能有一个，别新建", "VALIDATION")
	}
	password := bodyString(body, "password", 100)
	if len(password) < 6 {
		return apiError(400, "密码至少 6 位", "VALIDATION")
	}

	defaultShare := 0.0
	if role == "booster" {
		defaultShare = 60
	}
	user := User{
		ID: "usr_" + randomHex(6), Username: username,
		Name: firstNonEmpty(bodyString(body, "name", 30), username),
		Role: role, Phone: bodyString(body, "phone", 30),
		Share:        int(bodyNum(body, "share", defaultShare)),
		Status:       "on",
		PasswordHash: hashPassword(password), CreatedAt: nowISO(),
	}
	if body["status"] == "off" {
		user.Status = "off"
	}
	if user.Share < 0 {
		user.Share = 0
	}
	if user.Share > 100 {
		user.Share = 100
	}

	var problem *APIError
	_ = Update(func(db *DB) {
		if findUser(db, username) != nil {
			problem = apiError(400, "这个账号名已经有人用了", "USERNAME_TAKEN")
			return
		}
		db.Users = append(db.Users, user)
		addLog(db, fmt.Sprintf("%s 新建%s账号 %s", actor.Name, roleNames[role], username))
	})
	if problem != nil {
		return problem
	}
	okCreated(c.w, userJSON(&user))
	return nil
}

func handleUpdateUser(c *ctx, body map[string]any) error {
	db, actor, err := currentUser(c)
	if err != nil {
		return err
	}
	target := userByID(db, c.params["id"])
	if target == nil {
		return apiError(404, "账号不存在", "USER_NOT_FOUND")
	}
	newRole := firstNonEmpty(bodyString(body, "role", 10), target.Role)
	if problem := manageAllowed(actor, target.Role, newRole); problem != nil {
		return problem
	}
	if target.Role == "owner" && actor.Role != "owner" {
		return apiError(403, "董事长账号不能动", "FORBIDDEN")
	}
	if _, ok := roleNames[newRole]; !ok {
		return apiError(400, "角色不对", "VALIDATION")
	}
	if target.Role == "owner" && newRole != "owner" {
		return apiError(400, "不能把董事长改成别的角色", "VALIDATION")
	}
	password := bodyString(body, "password", 100)
	if password != "" && len(password) < 6 {
		return apiError(400, "新密码至少 6 位", "VALIDATION")
	}

	var saved *User
	var problem *APIError
	_ = Update(func(db *DB) {
		u := userByID(db, target.ID)
		if u == nil {
			problem = apiError(404, "账号不存在", "USER_NOT_FOUND")
			return
		}
		if _, ok := body["name"]; ok {
			u.Name = firstNonEmpty(bodyString(body, "name", 30), u.Username)
		}
		if _, ok := body["phone"]; ok {
			u.Phone = bodyString(body, "phone", 30)
		}
		if _, ok := body["role"]; ok {
			u.Role = newRole
		}
		if _, ok := body["share"]; ok {
			u.Share = int(bodyNum(body, "share", float64(u.Share)))
			if u.Share < 0 {
				u.Share = 0
			}
			if u.Share > 100 {
				u.Share = 100
			}
		}
		if _, ok := body["status"]; ok {
			if u.Role == "owner" && body["status"] == "off" {
				problem = apiError(400, "不能停用董事长账号", "VALIDATION")
				return
			}
			if body["status"] == "off" {
				u.Status = "off"
			} else {
				u.Status = "on"
			}
		}
		if password != "" {
			u.PasswordHash = hashPassword(password)
		}
		addLog(db, actor.Name+" 更新账号 "+u.Username)
		saved = u
	})
	if problem != nil {
		return problem
	}
	okData(c.w, userJSON(saved))
	return nil
}

func handleDeleteUser(c *ctx, _ map[string]any) error {
	db, actor, err := currentUser(c)
	if err != nil {
		return err
	}
	target := userByID(db, c.params["id"])
	if target == nil {
		return apiError(404, "账号不存在", "USER_NOT_FOUND")
	}
	if target.ID == actor.ID {
		return apiError(400, "不能删自己", "VALIDATION")
	}
	if target.Role == "owner" {
		return apiError(400, "董事长账号不能删", "VALIDATION")
	}
	_ = Update(func(db *DB) {
		for i := range db.Users {
			if db.Users[i].ID == target.ID {
				db.Users = append(db.Users[:i], db.Users[i+1:]...)
				break
			}
		}
		for i := range db.Orders {
			if db.Orders[i].AssigneeID == target.ID && db.Orders[i].Status != "done" {
				db.Orders[i].AssigneeID = ""
				db.Orders[i].AssigneeName = ""
			}
		}
		addLog(db, "删除账号 "+target.Username)
	})
	okData(c.w, map[string]any{"id": target.ID})
	return nil
}

func handleAdminLogs(c *ctx, _ map[string]any) error {
	db, _, err := currentUser(c)
	if err != nil {
		return err
	}
	logs := db.Logs
	if len(logs) > 80 {
		logs = logs[:80]
	}
	okData(c.w, map[string]any{"items": logs})
	return nil
}

func handleAssignOrder(c *ctx, body map[string]any) error {
	db, actor, err := currentUser(c)
	if err != nil {
		return err
	}
	oid := c.params["id"]
	uidValue := strings.TrimSpace(bodyString(body, "userId", 40))
	var target *User
	if uidValue != "" {
		target = userByID(db, uidValue)
		if target == nil || target.Role != "booster" || target.Status == "off" {
			return apiError(400, "只能派给在岗的打手", "VALIDATION")
		}
	}
	var saved *Order
	var problem *APIError
	_ = Update(func(db *DB) {
		var o *Order
		for i := range db.Orders {
			if db.Orders[i].ID == oid || db.Orders[i].OrderNo == oid {
				o = &db.Orders[i]
				break
			}
		}
		if o == nil {
			problem = apiError(404, "订单不存在", "ORDER_NOT_FOUND")
			return
		}
		if target != nil {
			o.AssigneeID = target.ID
			o.AssigneeName = target.Name
			o.AssignedAt = nowISO()
			if o.Status == "pending" && firstNonEmpty(o.PayStatus, "unpaid") == "paid" {
				o.Status = "processing"
			}
		} else {
			o.AssigneeID = ""
			o.AssigneeName = ""
			o.AssignedAt = ""
		}
		o.UpdatedAt = nowISO()
		addLog(db, fmt.Sprintf("%s 派单 %s → %s", actor.Name, o.OrderNo, firstNonEmpty(func() string {
			if target != nil {
				return target.Name
			}
			return ""
		}(), "未分配")))
		saved = o
	})
	if problem != nil {
		return problem
	}
	okData(c.w, saved)
	return nil
}
