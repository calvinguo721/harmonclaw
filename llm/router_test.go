package llm

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouter_FilterBySovereignty(t *testing.T) {
	backends := []BackendConfig{
		{ID: "a", Sovereignty: []string{"shadow"}},
		{ID: "b", Sovereignty: []string{"airlock", "opensea"}},
		{ID: "c", Sovereignty: []string{"opensea"}},
	}
	out := filterBySovereignty(backends, "shadow")
	if len(out) != 1 || out[0].ID != "a" {
		t.Errorf("shadow: got %v", out)
	}
	out = filterBySovereignty(backends, "airlock")
	if len(out) != 1 || out[0].ID != "b" {
		t.Errorf("airlock: got %v", out)
	}
	out = filterBySovereignty(backends, "opensea")
	if len(out) != 2 {
		t.Errorf("opensea: got %v", out)
	}
}

func TestRouter_Chat_MockBackend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"hello"}}]}`))
	}))
	defer srv.Close()

	r := NewRouter()
	r.mu.Lock()
	r.backends = []BackendConfig{
		{ID: "mock", Endpoint: srv.URL, Model: "test", EnvKey: "", Sovereignty: []string{"airlock"}},
	}
	r.client = srv.Client()
	r.mu.Unlock()

	resp, err := r.Chat(Request{Messages: []Message{{Role: "user", Content: "hi"}}}, "airlock")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "hello" {
		t.Errorf("content=%q", resp.Content)
	}
}

func TestRouter_Chat_NoBackend(t *testing.T) {
	r := NewRouter()
	r.mu.Lock()
	r.backends = []BackendConfig{
		{ID: "needkey", Endpoint: "http://x", EnvKey: "NEVER_SET_KEY", Sovereignty: []string{"airlock"}},
	}
	r.mu.Unlock()

	_, err := r.Chat(Request{}, "airlock")
	if err == nil {
		t.Error("expected error when no backend available")
	}
}
