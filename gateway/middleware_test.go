package gateway

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"harmonclaw/governor"
	"harmonclaw/viking"
)

func TestChain_RequestFlow(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-mw-test")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	ledger, _ := viking.NewFileLedger(dir)
	defer ledger.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	})
	h := Chain(mux, ledger, nil, nil, false)
	req := httptest.NewRequest("GET", "/v1/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Errorf("Chain: want 200, got %d", rr.Code)
	}
}

func TestChain_ShadowMode(t *testing.T) {
	old := SovereigntyMode
	SovereigntyMode = "shadow"
	defer func() { SovereigntyMode = old }()
	dir := filepath.Join(os.TempDir(), "harmonclaw-mw-shadow")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	ledger, _ := viking.NewFileLedger(dir)
	defer ledger.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/x", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := Chain(mux, ledger, nil, nil, false)
	req := httptest.NewRequest("GET", "/v1/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 403 {
		t.Errorf("shadow: want 403, got %d", rr.Code)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	cfg, _ := governor.LoadRateLimitConfig("configs/ratelimit.json")
	rl := governor.NewTripleRateLimiter(cfg)
	dir := filepath.Join(os.TempDir(), "harmonclaw-mw-rl")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	ledger, _ := viking.NewFileLedger(dir)
	defer ledger.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/x", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := Chain(mux, ledger, nil, rl, false)
	req := httptest.NewRequest("GET", "/v1/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Errorf("ratelimit allow: want 200, got %d", rr.Code)
	}
}

func init() {
	_ = os.Chdir
}
