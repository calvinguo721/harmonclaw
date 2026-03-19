// Package smoke provides end-to-end HTTP smoke tests for HarmonClaw.
package smoke

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"harmonclaw/architect"
	"harmonclaw/bus"
	"harmonclaw/butler"
	"harmonclaw/gateway"
	"harmonclaw/governor"
	"harmonclaw/governor/ironclaw"
	"harmonclaw/llm"
	"harmonclaw/sandbox"
	"harmonclaw/skills"
	"harmonclaw/viking"
	hctest "harmonclaw/pkg/testutil"

	_ "harmonclaw/skills/doc_perceiver"
	_ "harmonclaw/skills/openclaw_adapter"
	_ "harmonclaw/skills/web_search"
	_ "harmonclaw/skills/tts"
)

const baseURL = "http://127.0.0.1:18080"

func TestSmoke(t *testing.T) {
	os.Setenv("DEEPSEEK_API_KEY", "")
	os.Setenv("HC_AUTH_ENABLED", "")
	ledgerDir := filepath.Join(os.TempDir(), "harmonclaw-smoke-ledger")
	os.MkdirAll(ledgerDir, 0755)
	ledger, err := viking.NewFileLedger(ledgerDir)
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	defer ledger.Close()

	vikingDir := filepath.Join(os.TempDir(), "harmonclaw-smoke-viking")
	os.MkdirAll(vikingDir, 0755)
	mem, _ := viking.NewFileStore(vikingDir)
	guard := sandbox.NewWhitelist()
	policies, _ := ironclaw.LoadPolicies(hctest.ConfigPath("policies.json"))
	governor.InitSecureClient(ledger, "airlock", []string{"*"})
	gateway.SovereigntyMode = "airlock"

	provider, _ := llm.NewProvider()
	gov := governor.New(ledger)
	b := butler.New(provider, mem, ledger)
	a := architect.New(guard, ledger)
	b.SetGrantFunc(gov.RequestGrant)
	a.SetGrantFunc(gov.RequestGrant)
	a.Pool().Start()

	go gov.Pulse()
	go b.Pulse()
	go a.Pulse()

	lastPulse := map[bus.CoreID]time.Time{bus.Governor: time.Now(), bus.Butler: time.Now(), bus.Architect: time.Now()}
	go func() {
		ch := bus.Subscribe()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case m := <-ch:
				if m.Type == "pulse" {
					lastPulse[m.From] = time.Now()
					switch m.From {
					case bus.Governor:
						gov.SetOK()
					case bus.Butler:
						b.SetOK()
					case bus.Architect:
						a.SetOK()
					}
				}
			case <-ticker.C:
				now := time.Now()
				for core, tm := range lastPulse {
					if now.Sub(tm) > 15*time.Second {
						switch core {
						case bus.Governor:
							gov.SetDegraded()
						case bus.Butler:
							b.SetDegraded()
						case bus.Architect:
							a.SetDegraded()
						}
					}
				}
			}
		}
	}()

	srv := gateway.NewWithEngramDir(":18080", gov, b, a, ledger, policies, "v0.1.7-test", vikingDir)
	srv.VikingStore = viking.NewKVStore()
	srv.VikingSearch = viking.NewSearchIndexWithPath(filepath.Join(vikingDir, "index.jsonl"))
	snapDir := filepath.Join(vikingDir, "snapshots")
	srv.VikingSnap = viking.NewSnapshotManager(snapDir, vikingDir, 24)
	go func() {
		_ = srv.ListenAndServe()
	}()

	for i := 0; i < 30; i++ {
		resp, err := http.Get(baseURL + "/v1/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		if i == 29 {
			t.Fatal("server did not become ready")
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Run("health", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/health")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("health: want 200, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		var d map[string]any
		if json.Unmarshal(body, &d) != nil {
			t.Fatal("health: invalid JSON")
		}
		if d["overall"] == nil {
			t.Error("health: missing overall")
		}
		if d["governor"] == nil || d["butler"] == nil || d["architect"] == nil {
			t.Error("health: missing core status")
		}
		if d["architect"] != nil {
			if arch, ok := d["architect"].(map[string]any); ok && arch["registered_skills"] == nil {
				t.Error("health: missing registered_skills")
			}
		}
	})

	t.Run("chat", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
		resp, err := http.Post(baseURL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("chat: want 200, got %d", resp.StatusCode)
		}
	})

	t.Run("skills", func(t *testing.T) {
		body := []byte(`{"skill_name":"web_search","text":"test","args":{"q":"test"}}`)
		resp, err := http.Post(baseURL+"/v1/skills/execute", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 403 && resp.StatusCode != 502 {
			t.Errorf("skills: want 200/403/502, got %d", resp.StatusCode)
		}
	})

	t.Run("engram", func(t *testing.T) {
		body := []byte(`{"text":"hello","source":"user","classification":"public"}`)
		resp, err := http.Post(baseURL+"/v1/engram/inject", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("engram: want 200, got %d: %s", resp.StatusCode, mustRead(resp.Body))
		}
	})

	t.Run("ledger", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/ledger/latest")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("ledger: want 200, got %d: %s", resp.StatusCode, mustRead(resp.Body))
		}
	})

	t.Run("ledger_limit", func(t *testing.T) {
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get(baseURL + "/v1/ledger/latest?limit=5")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("ledger limit: want 200, got %d", resp.StatusCode)
		}
	})

	t.Run("sovereignty_get", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/governor/sovereignty")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("sovereignty GET: want 200, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		var d map[string]any
		if json.Unmarshal(body, &d) != nil {
			t.Fatal("sovereignty: invalid JSON")
		}
		if d["mode"] == nil || d["domains"] == nil {
			t.Error("sovereignty: missing mode or domains")
		}
	})

	t.Run("sovereignty_post", func(t *testing.T) {
		body := []byte(`{"mode":"airlock","domains":["api.deepseek.com"]}`)
		resp, err := http.Post(baseURL+"/v1/governor/sovereignty", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("sovereignty POST: want 200, got %d: %s", resp.StatusCode, mustRead(resp.Body))
		}
	})

	t.Run("chat_sse", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":"hi"}],"stream":true}`)
		resp, err := http.Post(baseURL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("chat SSE: want 200, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
			t.Errorf("chat SSE: want text/event-stream, got %s", ct)
		}
		// consume at least one chunk
		buf := make([]byte, 256)
		n, _ := resp.Body.Read(buf)
		if n == 0 {
			t.Error("chat SSE: expected at least one byte")
		}
	})

	t.Run("panic_recovery", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/test/panic")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 500 {
			t.Errorf("panic recovery: want 500, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		var d map[string]string
		if json.Unmarshal(body, &d) == nil && d["error"] != "internal server error" {
			t.Errorf("panic recovery: want error message, got %v", d)
		}
	})

	t.Run("debug_vars", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/debug/vars")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("debug/vars: want 200, got %d", resp.StatusCode)
		}
	})

	t.Run("audit_query", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/audit/query")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 501 && resp.StatusCode != 500 {
			t.Errorf("audit query: want 200/501/500, got %d", resp.StatusCode)
		}
	})

	t.Run("persona_get", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/butler/persona")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("persona GET: want 200, got %d", resp.StatusCode)
		}
	})

	t.Run("architect_skills", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/architect/skills")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("architect skills: want 200, got %d", resp.StatusCode)
		}
	})

	t.Run("architect_pipeline", func(t *testing.T) {
		body := []byte(`{"stages":[{"skill_id":"web_search","args":{"q":"test"}}]}`)
		resp, err := http.Post(baseURL+"/v1/architect/pipeline/execute", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 502 {
			t.Errorf("pipeline: want 200/502, got %d", resp.StatusCode)
		}
	})

	t.Run("architect_crons", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/architect/crons")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("architect crons: want 200, got %d", resp.StatusCode)
		}
	})

	t.Run("viking_snapshots", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/viking/snapshots")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 501 {
			t.Errorf("viking snapshots: want 200/501, got %d", resp.StatusCode)
		}
	})

	t.Run("viking_search", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/viking/search?q=test")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 501 {
			t.Errorf("viking search: want 200/501, got %d", resp.StatusCode)
		}
	})

	t.Run("token", func(t *testing.T) {
		body := []byte(`{"user_id":"smoke"}`)
		resp, err := http.Post(baseURL+"/v1/token", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("token: want 200, got %d: %s", resp.StatusCode, mustRead(resp.Body))
			return
		}
		raw, _ := io.ReadAll(resp.Body)
		var d map[string]any
		if json.Unmarshal(raw, &d) == nil && d["token"] == nil {
			t.Error("token: missing token in response")
		}
	})

	t.Run("auth_login", func(t *testing.T) {
		body := []byte(`{"username":"smoke","password":"x"}`)
		resp, err := http.Post(baseURL+"/v1/auth/login", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("auth login: want 200, got %d: %s", resp.StatusCode, mustRead(resp.Body))
		}
	})

	t.Run("ledger_trace", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/ledger/trace?action_id=00000000-0000-0000-0000-000000000000")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 400 {
			t.Errorf("ledger trace: want 200/400, got %d", resp.StatusCode)
		}
	})

	t.Run("persona_post", func(t *testing.T) {
		body := []byte(`{"id":"smoke","persona":{"system_prompt":"test"},"default":"smoke"}`)
		resp, err := http.Post(baseURL+"/v1/butler/persona", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 501 {
			t.Errorf("persona POST: want 200/501, got %d", resp.StatusCode)
		}
	})

	t.Run("viking_search_post", func(t *testing.T) {
		body := []byte(`{"query":"test"}`)
		resp, err := http.Post(baseURL+"/v1/viking/search", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 501 {
			t.Errorf("viking search POST: want 200/501, got %d", resp.StatusCode)
		}
	})

	t.Run("test_illegal", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/test/illegal")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Errorf("test illegal: want 403, got %d", resp.StatusCode)
		}
	})
}

func mustRead(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}

func init() {
	_ = skills.Registry
}
