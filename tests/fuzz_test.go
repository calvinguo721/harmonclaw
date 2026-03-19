package tests

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"harmonclaw/governor"
	"harmonclaw/viking"
)

func FuzzFirewall_Path(f *testing.F) {
	f.Add("/v1/health")
	f.Add("/v1/../etc/passwd")
	f.Add("/v1/..%2fadmin")
	f.Fuzz(func(t *testing.T, path string) {
		if len(path) > 200 {
			return
		}
		dir := t.TempDir()
		ledgerDir := filepath.Join(dir, "ledger")
		os.MkdirAll(ledgerDir, 0755)
		ledger, err := viking.NewFileLedger(ledgerDir)
		if err != nil {
			return
		}
		defer ledger.Close()
		fw := governor.NewFirewall(ledger)
		handler := fw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
		}))
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		_ = rr.Code
	})
}

func FuzzFirewallConfig_ContainsPathTraversal(f *testing.F) {
	cfg := governor.DefaultFirewallConfig()
	f.Add("../")
	f.Add("normal")
	f.Add("..%2f")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1000 {
			return
		}
		_ = cfg.ContainsPathTraversal(s)
	})
}
