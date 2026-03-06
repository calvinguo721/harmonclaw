// Package governor (firewall) provides request firewall: body limit, Content-Type, IP rate ban.
package governor

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"harmonclaw/viking"
)

const (
	maxBodyBytes     = 1 << 20 // 1MB
	maxRequestsPerIP = 20
	banDuration      = 60 * time.Second
)

// Firewall wraps handler with body limit, Content-Type check, IP rate ban.
type Firewall struct {
	ledger   viking.Ledger
	ipCounts map[string]*ipEntry
	ipBans   map[string]time.Time
	mu       sync.RWMutex
}

type ipEntry struct {
	count    int
	windowAt time.Time
}

func NewFirewall(ledger viking.Ledger) *Firewall {
	f := &Firewall{
		ledger:   ledger,
		ipCounts: make(map[string]*ipEntry),
		ipBans:   make(map[string]time.Time),
	}
	go f.cleanupLoop()
	return f
}

func (f *Firewall) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		f.mu.Lock()
		now := time.Now()
		for ip, until := range f.ipBans {
			if now.After(until) {
				delete(f.ipBans, ip)
			}
		}
		for ip, e := range f.ipCounts {
			if now.Sub(e.windowAt) > time.Second {
				delete(f.ipCounts, ip)
			}
		}
		f.mu.Unlock()
	}
}

func (f *Firewall) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if f.isBanned(ip) {
			f.recordBan(ip, "rate_exceeded")
			http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
			ct := r.Header.Get("Content-Type")
			if r.ContentLength > 0 && ct != "" && !strings.Contains(ct, "application/json") && !strings.Contains(ct, "text/") && !strings.Contains(ct, "multipart/") {
				http.Error(w, `{"error":"invalid content-type"}`, http.StatusUnsupportedMediaType)
				return
			}
		}
		if !f.recordRequest(ip) {
			f.ban(ip)
			f.recordBan(ip, "rate_exceeded")
			http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		if idx := strings.Index(x, ","); idx >= 0 {
			return strings.TrimSpace(x[:idx])
		}
		return strings.TrimSpace(x)
	}
	if idx := strings.Index(r.RemoteAddr, ":"); idx >= 0 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}

func (f *Firewall) isBanned(ip string) bool {
	f.mu.RLock()
	until, ok := f.ipBans[ip]
	f.mu.RUnlock()
	return ok && time.Now().Before(until)
}

func (f *Firewall) recordRequest(ip string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.ipCounts[ip]
	if e == nil {
		f.ipCounts[ip] = &ipEntry{count: 1, windowAt: time.Now()}
		return true
	}
	if time.Since(e.windowAt) > time.Second {
		e.count = 1
		e.windowAt = time.Now()
		return true
	}
	e.count++
	return e.count <= maxRequestsPerIP
}

func (f *Firewall) ban(ip string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ipBans[ip] = time.Now().Add(banDuration)
}

func (f *Firewall) recordBan(ip, reason string) {
	if f.ledger != nil {
		f.ledger.Record(viking.LedgerEntry{
			OperatorID: "governor",
			ActionType: "firewall_ban",
			Resource:   ip,
			Result:     "fail",
			ClientIP:   ip,
			Timestamp:  time.Now().Format(time.RFC3339),
			ActionID:   reason,
		})
	}
}
