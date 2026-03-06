package governor

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	tokenSecret []byte
	tokenOnce   sync.Once
)

func initTokenSecret() {
	tokenOnce.Do(func() {
		s := os.Getenv("HC_TOKEN_SECRET")
		if s != "" {
			tokenSecret = []byte(s)
			return
		}
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			tokenSecret = make([]byte, 32)
			return
		}
		tokenSecret = b
		log.Printf("governor: HC_TOKEN_SECRET not set, using random key: %s (DO NOT use in production)", hex.EncodeToString(b[:8]))
	})
}

type Claims struct {
	UserID   string `json:"user_id"`
	IssuedAt int64  `json:"issued_at"`
	ExpiresAt int64 `json:"expires_at"`
	Scope   string `json:"scope"`
}

func GenerateToken(userID string) (string, error) {
	initTokenSecret()
	now := time.Now().Unix()
	claims := Claims{
		UserID:    userID,
		IssuedAt:  now,
		ExpiresAt: now + 3600,
		Scope:     "default",
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	b64 := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, tokenSecret)
	mac.Write([]byte(b64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return b64 + "." + sig, nil
}

func ValidateToken(token string) (Claims, error) {
	initTokenSecret()
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Claims{}, fmt.Errorf("invalid token format")
	}
	b64, sigB64 := parts[0], parts[1]

	mac := hmac.New(sha256.New, tokenSecret)
	mac.Write([]byte(b64))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if sigB64 != expected {
		return Claims{}, fmt.Errorf("invalid signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return Claims{}, err
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return Claims{}, err
	}
	if time.Now().Unix() > c.ExpiresAt {
		return Claims{}, fmt.Errorf("token expired")
	}
	return c, nil
}
