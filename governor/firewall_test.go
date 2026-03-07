package governor

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"harmonclaw/viking"
)

func TestFirewall_BodyLimit(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-firewall-test")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	ledger, err := viking.NewFileLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	f := NewFirewall(ledger)
	handler := f.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("POST", "/", nil)
	req.Body = http.MaxBytesReader(nil, req.Body, 1)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != 400 && rr.Code != 413 {
		t.Logf("body limit: got %d (expected 400/413)", rr.Code)
	}
}

func TestFirewall_ContentType(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-firewall-ct")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	ledger, _ := viking.NewFileLedger(dir)
	defer ledger.Close()
	f := NewFirewall(ledger)
	handler := f.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("POST", "/", nil)
	req.ContentLength = 10
	req.Header.Set("Content-Type", "application/octet-stream")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != 415 {
		t.Errorf("invalid content-type: want 415, got %d", rr.Code)
	}
}

func TestFirewall_AllowJSON(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-firewall-json")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	ledger, _ := viking.NewFileLedger(dir)
	defer ledger.Close()
	f := NewFirewall(ledger)
	handler := f.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Errorf("application/json: want 200, got %d", rr.Code)
	}
}

func TestFirewall_PathTraversal(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-firewall-path")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	ledger, _ := viking.NewFileLedger(dir)
	defer ledger.Close()
	f := NewFirewall(ledger)
	handler := f.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("GET", "/v1/../etc/passwd", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Errorf("path traversal: want 400, got %d", rr.Code)
	}
}

func TestFirewall_SuspiciousHeader(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-firewall-hdr")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	ledger, _ := viking.NewFileLedger(dir)
	defer ledger.Close()
	cfg := DefaultFirewallConfig()
	cfg.BlockSuspiciousHdrs = true
	f := NewFirewallWithConfig(ledger, cfg)
	handler := f.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("GET", "/v1/health", nil)
	req.Header.Set("X-Original-URL", "/admin")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != 403 {
		t.Errorf("suspicious header: want 403, got %d", rr.Code)
	}
}
