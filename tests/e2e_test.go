package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"harmonclaw/architect"
	"harmonclaw/butler"
	"harmonclaw/gateway"
	"harmonclaw/governor"
	"harmonclaw/sandbox"
	"harmonclaw/viking"

	_ "harmonclaw/skills/doc_perceiver"
	_ "harmonclaw/skills/web_search"
)

func TestE2E_HealthToChatToSkill(t *testing.T) {
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"E2E reply"}}]}`))
	}))
	defer mockLLM.Close()

	provider := newMockProvider(mockLLM.URL)
	dir := t.TempDir()
	ledgerDir := filepath.Join(dir, "ledger")
	vikingDir := filepath.Join(dir, "viking")
	os.MkdirAll(ledgerDir, 0755)
	os.MkdirAll(vikingDir, 0755)

	ledger, err := viking.NewFileLedger(ledgerDir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()

	mem, _ := viking.NewFileStore(vikingDir)
	guard := sandbox.NewWhitelist()
	gov := governor.New(ledger)
	b := butler.NewWithOpts(provider, mem, ledger, vikingDir, "")
	a := architect.New(guard, ledger)
	b.SetGrantFunc(gov.RequestGrant)
	a.SetGrantFunc(gov.RequestGrant)
	a.Pool().Start()

	srv := gateway.NewWithEngramDir(":0", gov, b, a, ledger, nil, "test", vikingDir)
	srv.SetFirewall(governor.NewFirewall(ledger))
	governor.InitSecureClient(ledger, "airlock", []string{"*"})
	gateway.SovereigntyMode = "airlock"

	ts := httptest.NewServer(gateway.Chain(srv.Mux, ledger, srv.Firewall, nil, false))
	defer ts.Close()

	// 1. Health
	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("health: want 200, got %d", resp.StatusCode)
	}

	// 2. Chat
	body, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	resp, err = http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("chat: want 200, got %d", resp.StatusCode)
	}

	// 3. Skill
	body, _ = json.Marshal(map[string]any{"skill_id": "doc_perceiver", "text": "hello"})
	resp, err = http.Post(ts.URL+"/v1/skills/execute", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("skill: want 200, got %d", resp.StatusCode)
	}
}
