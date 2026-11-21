package main

// 数据结构：JSON 字段和 Python 版保持一致，两端可以互相接管 data/db.json。

type Meta struct {
	Version   int    `json:"version"`
	Brand     string `json:"brand"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type Admin struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
	DisplayName  string `json:"displayName"`
	LastLoginAt  string `json:"lastLoginAt"`
}

type HeroStat struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Suffix string `json:"suffix"`
}

type Kefu struct {
	QQ     string `json:"qq"`
	WeChat string `json:"wechat"`
	Phone  string `json:"phone"`
	Hours  string `json:"hours"`
}

type Settings struct {
	SiteName    string     `json:"siteName"`
	Company     string     `json:"company"`
	Slogan      string     `json:"slogan"`
	SubSlogan   string     `json:"subSlogan"`
	Notice      string     `json:"notice"`
	HeroStats   []HeroStat `json:"heroStats"`
	Kefu        Kefu       `json:"kefu"`
	Copyright   string     `json:"copyright"`
	ICP         string     `json:"icp"`
	AdminNotice string     `json:"adminNotice"`
	OrderTips   string     `json:"orderTips"`
}

type Product struct {
	ID        int      `json:"id"`
	Game      string   `json:"game"`
	Category  string   `json:"category"`
	Title     string   `json:"title"`
	Subtitle  string   `json:"subtitle"`
	Cover     string   `json:"cover"`
	Price     float64  `json:"price"`
	OldPrice  float64  `json:"oldPrice"`
	Unit      string   `json:"unit"`
	Tags      []string `json:"tags"`
	Desc      string   `json:"desc"`
	Includes  []string `json:"includes"`
	Stock     int      `json:"stock"`
	Sales     int      `json:"sales"`
	Status    string   `json:"status"`
	Sort      int      `json:"sort"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

type Order struct {
	ID           string  `json:"id"`
	OrderNo      string  `json:"orderNo"`
	ProductID    int     `json:"productId"`
	ProductTitle string  `json:"productTitle"`
	Game         string  `json:"game"`
	Category     string  `json:"category"`
	Cover        string  `json:"cover"`
	Price        float64 `json:"price"`
	Unit         string  `json:"unit"`
	Qty          int     `json:"qty"`
	Amount       float64 `json:"amount"`
	ContactName  string  `json:"contactName"`
	ContactPhone string  `json:"contactPhone"`
	GameAccount  string  `json:"gameAccount"`
	Server       string  `json:"server"`
	Note         string  `json:"note"`
	Status       string  `json:"status"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
}

type Log struct {
	ID     string `json:"id"`
	At     string `json:"at"`
	Action string `json:"action"`
}

type DB struct {
	Meta     Meta      `json:"meta"`
	Admin    Admin     `json:"admin"`
	Settings Settings  `json:"settings"`
	Products []Product `json:"products"`
	Orders   []Order   `json:"orders"`
	Logs     []Log     `json:"logs"`
}

type Seed struct {
	Settings Settings  `json:"settings"`
	Products []Product `json:"products"`
}

type APIError struct {
	Status  int
	Message string
	Code    string
}

func (e *APIError) Error() string { return e.Message }

func apiError(status int, message, code string) *APIError {
	return &APIError{Status: status, Message: message, Code: code}
}
