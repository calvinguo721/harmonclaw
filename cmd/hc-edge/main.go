// Package main provides hc-edge lightweight client for RV2.
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"runtime"
	"sync"
	"time"
)

const (
	defaultBackend = "http://192.168.1.100:8080"
	edgePort       = "8081"
	healthInterval = 10 * time.Second
	cacheSize      = 100
)

var (
	backendURL string
	connected  bool
	connMu     sync.RWMutex
	cache      []cacheEntry
	cacheMu    sync.RWMutex
)

type cacheEntry struct {
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Time      time.Time `json:"time"`
}

func main() {
	backendURL = os.Getenv("HC_BACKEND_URL")
	if backendURL == "" {
		backendURL = defaultBackend
	}
	log.Printf("hc-edge: backend=%s port=%s", backendURL, edgePort)

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/api/cache", handleCache)
	http.HandleFunc("/api/", handleProxy)
	http.HandleFunc("/v1/", handleProxy)
	http.HandleFunc("/static/", handleProxy)
	http.HandleFunc("/debug/", handleProxy)

	go healthLoop()
	log.Fatal(http.ListenAndServe(":"+edgePort, nil))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" || r.URL.Path == "" {
		if f, err := os.Open("web/edge/index.html"); err == nil {
			io.Copy(w, f)
			f.Close()
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html><html><body><h1>HC Edge</h1><p>Backend: <a href="/health">/health</a></p></body></html>`))
		return
	}
	handleProxy(w, r)
}

func handleProxy(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse(backendURL)
	if err != nil {
		http.Error(w, "invalid backend", 500)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = r.URL.Path
		req.Host = target.Host
	}
	proxy.ServeHTTP(w, r)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	connMu.RLock()
	c := connected
	connMu.RUnlock()
	info := map[string]any{
		"connected": c,
		"arch":      runtime.GOARCH,
		"os":        runtime.GOOS,
		"backend":   backendURL,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func healthLoop() {
	ticker := time.NewTicker(healthInterval)
	defer ticker.Stop()
	for range ticker.C {
		resp, err := http.Get(backendURL + "/v1/health")
		connMu.Lock()
		connected = err == nil && resp != nil && resp.StatusCode == 200
		connMu.Unlock()
		if resp != nil {
			resp.Body.Close()
		}
	}
}

func cacheAdd(sessionID, role, content string) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cache = append(cache, cacheEntry{SessionID: sessionID, Role: role, Content: content, Time: time.Now()})
	if len(cache) > cacheSize {
		cache = cache[len(cache)-cacheSize:]
	}
}

func handleCache(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cacheGet())
}

func cacheGet() []cacheEntry {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	out := make([]cacheEntry, len(cache))
	copy(out, cache)
	return out
}
