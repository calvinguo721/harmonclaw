// Package tests provides full integration tests.
package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

const baseURL = "http://127.0.0.1:18080"

// TestIntegrationFullFlow requires a running server on :18080. Run: go run ./cmd/harmonclaw (with port 18080).
// Or use: HC_PORT=18080 go run ./cmd/harmonclaw
func TestIntegrationFullFlow(t *testing.T) {
	if !serverReady(baseURL) {
		t.Skip("server not running at " + baseURL + ", skip integration test")
	}

	t.Run("login", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/v1/auth/login", "application/json", bytes.NewReader([]byte(`{"username":"test","password":"test"}`)))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 401 {
			t.Logf("login: %d (auth may be disabled)", resp.StatusCode)
		}
	})

	t.Run("chat", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
		resp, err := http.Post(baseURL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("chat: want 200, got %d", resp.StatusCode)
		}
	})

	t.Run("search", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/viking/search?q=test")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 400 {
			t.Errorf("search: want 200/400, got %d", resp.StatusCode)
		}
	})

	t.Run("audit", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/audit/query?limit=5")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("audit: want 200, got %d", resp.StatusCode)
		}
	})

	t.Run("snapshots", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/viking/snapshots")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("snapshots: want 200, got %d", resp.StatusCode)
		}
	})
}

func TestIntegrationConcurrent(t *testing.T) {
	if !serverReady(baseURL) {
		t.Skip("server not running, skip")
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _ := http.Get(baseURL + "/v1/health")
			if resp != nil {
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
}

func TestIntegrationRateLimit(t *testing.T) {
	if !serverReady(baseURL) {
		t.Skip("server not running, skip")
	}
	resp, err := http.Get(baseURL + "/v1/governor/ratelimit")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("ratelimit GET: want 200, got %d", resp.StatusCode)
	}
	var d map[string]any
	body, _ := io.ReadAll(resp.Body)
	if json.Unmarshal(body, &d) != nil {
		t.Fatal("ratelimit: invalid JSON")
	}
	if d["global"] == nil && d["per_user"] == nil {
		t.Error("ratelimit: missing config")
	}
}

func serverReady(url string) bool {
	for i := 0; i < 5; i++ {
		resp, err := http.Get(url + "/v1/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return true
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
