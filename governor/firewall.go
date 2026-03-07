// Package governor (firewall) provides request firewall: body limit, Content-Type, IP rate ban, path blocklist.
package governor

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"harmonclaw/viking"
)

// Firewall wraps handler with body limit, Content-Type check, IP rate ban, path blocklist.
type Firewall struct {
	ledger   viking.Ledger
	cfg      FirewallConfig
	ipCounts map[string]*ipEntry
	ipBans   map[string]time.Time
	mu       sync.RWMutex
}

type ipEntry struct {
	count    int
	windowAt time.Time
}

func NewFirewall(ledger viking.Ledger) *Firewall {
	return NewFirewallWithConfig(ledger, LoadFirewallConfig(""))
}

func NewFirewallWithConfig(ledger viking.Ledger, cfg FirewallConfig) *Firewall {
	f := &Firewall{
		ledger:   ledger,
		cfg:      cfg,
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

var suspiciousHeaders = []string{"X-Forwarded-Host", "X-Original-URL", "X-Rewrite-URL", "X-Custom-IP-Authorization"}

func (f *Firewall) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if f.isBanned(ip) {
			f.recordBan(ip, "rate_exceeded")
			http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		if f.cfg.ContainsPathTraversal(r.URL.Path) || f.cfg.ContainsPathTraversal(r.URL.RawQuery) {
			f.recordBan(ip, "path_traversal")
			http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
			return
		}
		if f.cfg.BlockSuspiciousHdrs {
			for _, h := range suspiciousHeaders {
				if r.Header.Get(h) != "" {
					f.recordBan(ip, "suspicious_header")
					http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
					return
				}
			}
		}
		maxBody := f.cfg.MaxBodyBytes
		if maxBody <= 0 {
			maxBody = 1 << 20
		}
		if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
			r.Body = http.MaxBytesReader(w, r.Body, int64(maxBody))
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
	maxReq := f.cfg.MaxRequestsPerIP
	if maxReq <= 0 {
		maxReq = 20
	}
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
	return e.count <= maxReq
}

func (f *Firewall) ban(ip string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ipBans[ip] = time.Now().Add(f.cfg.BanDuration())
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
