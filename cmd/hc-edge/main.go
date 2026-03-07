// Package main provides hc-edge lightweight client for RV2.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"harmonclaw/internal/edge"
)

const (
	defaultBackend = "http://192.168.1.100:8080"
	edgePort       = "8081"
	healthInterval = 10 * time.Second
	cacheSize      = 100
)

var (
	backendURL    string
	connected     bool
	connMu        sync.RWMutex
	cache         []cacheEntry
	cacheMu       sync.RWMutex
	offlineMgr    *edge.OfflineManager
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
	offlineMgr = edge.NewOfflineManager(backendURL)

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
	if offlineMgr != nil && offlineMgr.IsOffline() {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/chat") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(503)
			json.NewEncoder(w).Encode(map[string]string{"error": "离线模式，仅可查看历史"})
			return
		}
	}
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
	sessionID := "edge-" + time.Now().Format("20060102")
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/chat") {
		proxy.ModifyResponse = func(resp *http.Response) error {
			if resp.StatusCode == 200 && resp.Body != nil {
				var chat struct {
					Choices []struct {
						Message struct {
							Role    string `json:"role"`
							Content string `json:"content"`
						} `json:"message"`
					} `json:"choices"`
				}
				data, _ := io.ReadAll(resp.Body)
				resp.Body = io.NopCloser(io.Reader(nil))
				if json.Unmarshal(data, &chat) == nil && len(chat.Choices) > 0 {
					cacheAdd(sessionID, "assistant", chat.Choices[0].Message.Content)
					if offlineMgr != nil {
						offlineMgr.Record(sessionID, "assistant", chat.Choices[0].Message.Content)
					}
				}
				resp.Body = io.NopCloser(bytes.NewReader(data))
			}
			return nil
		}
	}
	proxy.ServeHTTP(w, r)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	connMu.RLock()
	c := connected
	connMu.RUnlock()
	offline := offlineMgr != nil && offlineMgr.IsOffline()
	info := map[string]any{
		"connected": c && !offline,
		"offline":   offline,
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
		if offlineMgr != nil {
			offlineMgr.Check()
			connMu.Lock()
			connected = !offlineMgr.IsOffline()
			connMu.Unlock()
		} else {
			resp, err := http.Get(backendURL + "/v1/health")
			connMu.Lock()
			connected = err == nil && resp != nil && resp.StatusCode == 200
			connMu.Unlock()
			if resp != nil {
				resp.Body.Close()
			}
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
