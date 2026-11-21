package main

// 密码和令牌。PBKDF2-HMAC-SHA256 与 Python 的 hashlib.pbkdf2_hmac 结果一致，
// 所以换了后端也不用重设管理员密码。

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	pbkdfIterations = 120000
	tokenTTLSeconds = 12 * 3600
)

var secretKey = func() string {
	if v := os.Getenv("SESSION_SECRET"); v != "" {
		return v
	}
	if v := os.Getenv("AUTH_SECRET"); v != "" {
		return v
	}
	return "wunding-escort-local-secret-please-change"
}()

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strings.Repeat("0", n*2)
	}
	return hex.EncodeToString(buf)
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	blocks := (keyLen + hashLen - 1) / hashLen
	dk := make([]byte, 0, blocks*hashLen)
	var counter [4]byte
	u := make([]byte, hashLen)
	for block := 1; block <= blocks; block++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(counter[:], uint32(block))
		prf.Write(counter[:])
		u = prf.Sum(u[:0])
		t := make([]byte, hashLen)
		copy(t, u)
		for n := 2; n <= iterations; n++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for i := range t {
				t[i] ^= u[i]
			}
		}
		dk = append(dk, t...)
	}
	return dk[:keyLen]
}

func hashPassword(password string) string {
	salt := randomHex(16)
	dk := pbkdf2SHA256([]byte(password), []byte(salt), pbkdfIterations, 32)
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s", pbkdfIterations, salt, hex.EncodeToString(dk))
}

func verifyPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	calc := pbkdf2SHA256([]byte(password), []byte(parts[2]), iter, 32)
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(calc)), []byte(parts[3])) == 1
}

func b64(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func signToken(user User) (string, error) {
	payload := map[string]any{
		"u":    user.Username,
		"role": user.Role,
		"uid":  user.ID,
		"name": user.Name,
		"exp":  time.Now().Unix() + tokenTTLSeconds,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	data := b64(raw)
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(data))
	return data + "." + b64(mac.Sum(nil)), nil
}

func verifyToken(token string) map[string]any {
	if token == "" || !strings.Contains(token, ".") {
		return nil
	}
	parts := strings.SplitN(token, ".", 2)
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(parts[0]))
	if !hmac.Equal([]byte(b64(mac.Sum(nil))), []byte(parts[1])) {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	exp, ok := payload["exp"].(float64)
	if !ok || int64(exp) < time.Now().Unix() {
		return nil
	}
	return payload
}
