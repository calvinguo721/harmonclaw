// Package tests provides conversation flow integration tests.
package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
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
	"harmonclaw/viking"

	_ "harmonclaw/skills/doc_perceiver"
	_ "harmonclaw/skills/web_search"
)

func TestConversationIntegration_FullFlow(t *testing.T) {
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/chat/completions" || r.URL.Path == "/chat/completions" {
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Hello from mock LLM"}}]}`))
		}
	}))
	defer mockLLM.Close()

	provider := newMockProvider(mockLLM.URL)
	dir := t.TempDir()
	ledgerDir := filepath.Join(dir, "ledger")
	os.MkdirAll(ledgerDir, 0755)
	ledger, err := viking.NewFileLedger(ledgerDir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()

	vikingDir := filepath.Join(dir, "viking")
	os.MkdirAll(vikingDir, 0755)
	mem, _ := viking.NewFileStore(vikingDir)
	governor.InitSecureClient(ledger, "airlock", []string{"*"})
	gateway.SovereigntyMode = "airlock"

	guard := sandbox.NewWhitelist()
	policies, _ := ironclaw.LoadPolicies("configs/policies.json")
	gov := governor.New(ledger)
	b := butler.NewWithOpts(provider, mem, ledger, vikingDir, "")
	a := architect.New(guard, ledger)
	b.SetGrantFunc(gov.RequestGrant)
	a.SetGrantFunc(gov.RequestGrant)
	a.Pool().Start()

	go gov.Pulse()
	go b.Pulse()
	go a.Pulse()

	srv := gateway.New(":0", gov, b, a, ledger, policies, "test")
	wrapped := gateway.Chain(srv.Mux, ledger, nil, nil, false)
	ts := httptest.NewServer(wrapped)
	defer ts.Close()

	t.Run("chat", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":"hello"}],"stream":false}`)
		resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			t.Errorf("chat: want 200, got %d: %s", resp.StatusCode, data)
		}
		var out struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		json.Unmarshal(data, &out)
		if len(out.Choices) > 0 {
			t.Logf("chat response: %s", out.Choices[0].Message.Content)
		}
	})

	t.Run("skill_execute", func(t *testing.T) {
		body := []byte(`{"skill_id":"doc_perceiver","input":{"trace_id":"t1","text":"test","args":{"path":"."}}}`)
		resp, err := http.Post(ts.URL+"/v1/skills/execute", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 400 {
			t.Errorf("skill: want 200/400, got %d", resp.StatusCode)
		}
	})

	t.Run("health", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/v1/health")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("health: want 200, got %d", resp.StatusCode)
		}
	})
}

func TestConversationIntegration_LLMFallback(t *testing.T) {
	failLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer failLLM.Close()

	provider := newMockProvider(failLLM.URL)
	dir := t.TempDir()
	ledger, _ := viking.NewFileLedger(filepath.Join(dir, "ledger"))
	defer ledger.Close()
	mem, _ := viking.NewFileStore(filepath.Join(dir, "viking"))
	governor.InitSecureClient(ledger, "airlock", []string{"*"})
	gateway.SovereigntyMode = "airlock"

	gov := governor.New(ledger)
	b := butler.New(provider, mem, ledger)
	a := architect.New(sandbox.NewWhitelist(), ledger)
	b.SetGrantFunc(gov.RequestGrant)
	a.SetGrantFunc(gov.RequestGrant)
	a.Pool().Start()

	srv := gateway.New(":0", gov, b, a, ledger, nil, "test")
	ts := httptest.NewServer(gateway.Chain(srv.Mux, ledger, nil, nil, false))
	defer ts.Close()

	body := []byte(`{"messages":[{"role":"user","content":"hi"}],"stream":false}`)
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 502 {
		t.Errorf("fallback: want 200 or 502, got %d", resp.StatusCode)
	}
}

func TestConversationIntegration_Concurrent(t *testing.T) {
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer mockLLM.Close()

	provider := newMockProvider(mockLLM.URL)
	dir := t.TempDir()
	ledger, _ := viking.NewFileLedger(filepath.Join(dir, "ledger"))
	defer ledger.Close()
	mem, _ := viking.NewFileStore(filepath.Join(dir, "viking"))
	governor.InitSecureClient(ledger, "airlock", []string{"*"})
	gateway.SovereigntyMode = "airlock"

	gov := governor.New(ledger)
	b := butler.New(provider, mem, ledger)
	a := architect.New(sandbox.NewWhitelist(), ledger)
	b.SetGrantFunc(gov.RequestGrant)
	a.SetGrantFunc(gov.RequestGrant)
	a.Pool().Start()

	srv := gateway.New(":0", gov, b, a, ledger, nil, "test")
	ts := httptest.NewServer(gateway.Chain(srv.Mux, ledger, nil, nil, false))
	defer ts.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := []byte(`{"messages":[{"role":"user","content":"hi"}],"stream":false}`)
			resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
			if err != nil {
				return
			}
			resp.Body.Close()
		}()
	}
	wg.Wait()
}

type mockProvider struct {
	baseURL string
	client  *http.Client
}

func newMockProvider(baseURL string) *mockProvider {
	return &mockProvider{baseURL: baseURL, client: &http.Client{Timeout: 5 * time.Second}}
}

func (m *mockProvider) Chat(req llm.Request) (llm.Response, error) {
	url := m.baseURL + "/v1/chat/completions"
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out struct {
		Choices []struct {
			Message llm.Message `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(data, &out) != nil || len(out.Choices) == 0 {
		return llm.Response{}, nil
	}
	return llm.Response{Content: out.Choices[0].Message.Content}, nil
}

func (m *mockProvider) ChatStream(req llm.Request) (<-chan string, error) {
	ch := make(chan string, 1)
	resp, err := m.Chat(req)
	if err != nil {
		close(ch)
		return ch, err
	}
	ch <- resp.Content
	close(ch)
	return ch, nil
}

func init() {
	_ = bus.Subscribe()
}
