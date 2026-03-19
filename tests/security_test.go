package tests

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"harmonclaw/gateway"
	"harmonclaw/governor"
	"harmonclaw/viking"
)

func TestSecurity_FirewallPathTraversal(t *testing.T) {
	dir := t.TempDir()
	ledgerDir := filepath.Join(dir, "ledger")
	os.MkdirAll(ledgerDir, 0755)
	ledger, err := viking.NewFileLedger(ledgerDir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()

	fw := governor.NewFirewall(ledger)
	handler := fw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	tests := []struct {
		path string
		want int
	}{
		{"/v1/../etc/passwd", 400},
		{"/v1/..%2fadmin", 400},
		{"/v1/health", 200},
	}
	for _, tt := range tests {
		req := httptest.NewRequest("GET", tt.path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != tt.want {
			t.Errorf("path %q: want %d, got %d", tt.path, tt.want, rr.Code)
		}
	}
}

func TestSecurity_SuspiciousHeader(t *testing.T) {
	dir := t.TempDir()
	ledgerDir := filepath.Join(dir, "ledger")
	os.MkdirAll(ledgerDir, 0755)
	ledger, err := viking.NewFileLedger(ledgerDir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()

	cfg := governor.DefaultFirewallConfig()
	cfg.BlockSuspiciousHdrs = true
	fw := governor.NewFirewallWithConfig(ledger, cfg)
	handler := fw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/v1/health", nil)
	req.Header.Set("X-Original-URL", "/admin")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != 403 {
		t.Errorf("suspicious header: want 403, got %d", rr.Code)
	}
}

func TestSecurity_ShadowModeBlocks(t *testing.T) {
	gateway.SovereigntyMode = "shadow"
	defer func() { gateway.SovereigntyMode = "airlock" }()

	dir := t.TempDir()
	ledgerDir := filepath.Join(dir, "ledger")
	os.MkdirAll(ledgerDir, 0755)
	ledger, _ := viking.NewFileLedger(ledgerDir)
	defer ledger.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	h := gateway.Chain(mux, ledger, nil, nil, false)
	req := httptest.NewRequest("GET", "/v1/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 403 {
		t.Errorf("shadow mode: want 403, got %d", rr.Code)
	}
}
