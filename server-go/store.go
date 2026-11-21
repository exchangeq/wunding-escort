package main

// 数据文件读写：全进程一把读写锁，落盘先写临时文件再 rename，
// 断电最多丢掉最后一次写入，不会留下半个 JSON。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

var (
	storeMu   sync.RWMutex
	storeOnce sync.Once
	currentDB *DB
	storeErr  error
	dataDir   = envOr("DATA_DIR", "data")
	dbFile    string
	uploadDir string
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func nowISO() string {
	return time.Now().Format("2006-01-02T15:04:05-07:00")
}

func orderNo() string {
	return "WD" + time.Now().Format("20060102") + strconv.Itoa(100000+randInt(900000))
}

func findSeedFile() (string, error) {
	candidates := []string{}
	if v := os.Getenv("SEED_FILE"); v != "" {
		candidates = append(candidates, v)
	}
	exe, _ := os.Executable()
	candidates = append(candidates, "seed.json")
	if exe != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "seed.json"))
	}
	candidates = append(candidates, filepath.Join("..", "seed.json"))
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("找不到 seed.json（可用 SEED_FILE 指定路径）")
}

func readSeed() (*Seed, error) {
	path, err := findSeedFile()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var seed Seed
	if err := json.Unmarshal(raw, &seed); err != nil {
		return nil, err
	}
	return &seed, nil
}

func initStore() {
	dbFile = filepath.Join(dataDir, "db.json")
	uploadDir = envOr("UPLOAD_DIR", filepath.Join(dataDir, "uploads"))
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		storeErr = err
		return
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		storeErr = err
		return
	}

	if raw, err := os.ReadFile(dbFile); err == nil {
		var db DB
		if err := json.Unmarshal(raw, &db); err == nil {
			normalizeDB(&db)
			currentDB = &db
			return
		}
		_ = os.WriteFile(dbFile+".broken", raw, 0o644)
	}

	seed, err := readSeed()
	if err != nil {
		storeErr = err
		return
	}
	db := &DB{
		Meta: Meta{Version: 2, Brand: "无定科技", CreatedAt: nowISO()},
		Admin: Admin{
			Username:     envOr("ADMIN_USER", "wunding"),
			PasswordHash: hashPassword(envOr("ADMIN_PASS", "wunding2026")),
			DisplayName:  "无定科技 · 运营",
		},
		Settings: seed.Settings,
		Logs:     []Log{{ID: "log_" + randomHex(8), At: nowISO(), Action: "初始化数据"}},
	}
	for i, p := range seed.Products {
		product := p
		product.ID = i + 1
		if product.Status == "" {
			product.Status = "on"
		}
		if product.Sort == 0 {
			product.Sort = i + 1
		}
		product.CreatedAt = nowISO()
		product.UpdatedAt = product.CreatedAt
		db.Products = append(db.Products, product)
	}
	if db.Orders == nil {
		db.Orders = []Order{}
	}
	normalizeDB(db)
	if err := persist(db); err != nil {
		storeErr = err
		return
	}
	currentDB = db
}

func normalizeDB(db *DB) {
	if db.Meta.Version == 0 {
		db.Meta.Version = 2
	}
	if db.Meta.Brand == "" {
		db.Meta.Brand = "无定科技"
	}
	if db.Meta.CreatedAt == "" {
		db.Meta.CreatedAt = nowISO()
	}
	if db.Products == nil {
		db.Products = []Product{}
	}
	if db.Orders == nil {
		db.Orders = []Order{}
	}
	if db.Logs == nil {
		db.Logs = []Log{}
	}
	if db.Admin.Username == "" {
		db.Admin.Username = "wunding"
		db.Admin.PasswordHash = hashPassword(envOr("ADMIN_PASS", "wunding2026"))
	}
	if db.Settings.Payment.Epay.Type == "" && db.Settings.Payment.Epay.Gateway == "" &&
		db.Settings.Payment.Epay.Pid == "" && db.Settings.Payment.Manual.Note == "" {
		db.Settings.Payment.Manual.Enabled = true
		db.Settings.Payment.Manual.Note = "付款时在备注里写上订单号，客服核对到账后排单。"
	}
	if db.Settings.Payment.Epay.Type == "" {
		db.Settings.Payment.Epay.Type = "alipay"
	}
	if db.Settings.Payment.Epay.Name == "" {
		db.Settings.Payment.Epay.Name = "在线支付"
	}
	if db.Settings.Copyright == "" {
		db.Settings.Copyright = "© 2026 无定科技 版权所有"
	}
}

func persist(db *DB) error {
	raw, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dataDir, "db-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	tmp.Close()
	return os.Rename(tmpName, dbFile)
}

func Read(fn func(*DB)) error {
	storeOnce.Do(initStore)
	if storeErr != nil {
		return storeErr
	}
	storeMu.RLock()
	defer storeMu.RUnlock()
	fn(currentDB)
	return nil
}

func Update(fn func(*DB)) error {
	storeOnce.Do(initStore)
	if storeErr != nil {
		return storeErr
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	fn(currentDB)
	currentDB.Meta.UpdatedAt = nowISO()
	return persist(currentDB)
}

func nextProductID(db *DB) int {
	max := 0
	for _, p := range db.Products {
		if p.ID > max {
			max = p.ID
		}
	}
	return max + 1
}

// addLog 只保留最近 200 条，避免文件越跑越大。
func addLog(db *DB, action string) {
	db.Logs = append([]Log{{ID: "log_" + randomHex(8), At: nowISO(), Action: action}}, db.Logs...)
	if len(db.Logs) > 200 {
		db.Logs = db.Logs[:200]
	}
}

func logAt(db *DB) string {
	if len(db.Logs) == 0 {
		return ""
	}
	return db.Logs[0].At
}

func sortedProducts(products []Product) []Product {
	out := make([]Product, len(products))
	copy(out, products)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Sort != out[j].Sort {
			return out[i].Sort < out[j].Sort
		}
		return out[i].ID > out[j].ID
	})
	return out
}
