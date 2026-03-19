// Package llm provides multi-model routing with sovereignty awareness.
package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"harmonclaw/governor"
)

// BackendConfig is a single LLM backend.
type BackendConfig struct {
	ID           string   `json:"id"`
	Endpoint     string   `json:"endpoint"`
	Model        string   `json:"model"`
	MaxTokens    int      `json:"max_tokens"`
	Temperature  float64  `json:"temperature"`
	EnvKey       string   `json:"env_key"`
	Sovereignty  []string `json:"sovereignty"`
}

// RouterConfig holds all backends.
type RouterConfig struct {
	Backends []BackendConfig `json:"backends"`
}

// Router routes requests to backends by sovereignty and failover.
type Router struct {
	mu       sync.RWMutex
	backends []BackendConfig
	client   *http.Client
}

// NewRouter creates a router. Loads from configs/llm.json if exists.
func NewRouter() *Router {
	r := &Router{
		backends: defaultBackends(),
		client:   governor.SecureClient(),
	}
	if data, err := os.ReadFile("configs/llm.json"); err == nil {
		var cfg RouterConfig
		if json.Unmarshal(data, &cfg) == nil && len(cfg.Backends) > 0 {
			r.backends = cfg.Backends
		}
	}
	return r
}

func defaultBackends() []BackendConfig {
	return []BackendConfig{
		{ID: "deepseek", Endpoint: "https://api.deepseek.com/v1/chat/completions", Model: "deepseek-chat", MaxTokens: 4096, Sovereignty: []string{"airlock", "opensea"}},
		{ID: "ollama", Endpoint: "http://localhost:11434/api/chat", Model: "llama2", MaxTokens: 2048, Sovereignty: []string{"shadow", "airlock", "opensea"}},
	}
}

// Chat sends request to first available backend for sovereignty mode.
func (r *Router) Chat(req Request, sovereignty string) (Response, error) {
	r.mu.RLock()
	backends := r.backends
	client := r.client
	r.mu.RUnlock()

	candidates := filterBySovereignty(backends, sovereignty)
	for _, b := range candidates {
		key := os.Getenv(b.EnvKey)
		if b.EnvKey != "" && key == "" {
			continue
		}
		resp, err := r.callBackend(client, b, req, false)
		if err == nil {
			return resp, nil
		}
	}
	return Response{}, fmt.Errorf("all backends failed for sovereignty %s", sovereignty)
}

// ChatStream streams from first available backend.
func (r *Router) ChatStream(req Request, sovereignty string) (<-chan string, error) {
	r.mu.RLock()
	backends := r.backends
	client := r.client
	r.mu.RUnlock()

	candidates := filterBySovereignty(backends, sovereignty)
	for _, b := range candidates {
		key := os.Getenv(b.EnvKey)
		if b.EnvKey != "" && key == "" {
			continue
		}
		ch, err := r.callBackendStream(client, b, req)
		if err == nil {
			return ch, nil
		}
	}
	ch := make(chan string, 1)
	ch <- "[router] LLM not available"
	close(ch)
	return ch, nil
}

func filterBySovereignty(backends []BackendConfig, mode string) []BackendConfig {
	if mode == "" {
		mode = "airlock"
	}
	var out []BackendConfig
	for _, b := range backends {
		for _, m := range b.Sovereignty {
			if m == mode {
				out = append(out, b)
				break
			}
		}
	}
	return out
}

func (r *Router) callBackend(client *http.Client, b BackendConfig, req Request, stream bool) (Response, error) {
	if req.Model == "" {
		req.Model = b.Model
	}
	body := map[string]any{
		"model":       req.Model,
		"messages":    req.Messages,
		"max_tokens":  b.MaxTokens,
		"temperature": b.Temperature,
		"stream":      false,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}
	httpReq, err := http.NewRequest(http.MethodPost, b.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if b.EnvKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+os.Getenv(b.EnvKey))
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer httpResp.Body.Close()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, err
	}
	if httpResp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("backend %s: %d %s", b.ID, httpResp.StatusCode, respBody)
	}
	var apiResp struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return Response{}, err
	}
	if len(apiResp.Choices) == 0 {
		return Response{}, fmt.Errorf("backend %s: no choices", b.ID)
	}
	return Response{Content: apiResp.Choices[0].Message.Content}, nil
}

func (r *Router) callBackendStream(client *http.Client, b BackendConfig, req Request) (<-chan string, error) {
	if req.Model == "" {
		req.Model = b.Model
	}
	body := map[string]any{
		"model":       req.Model,
		"messages":    req.Messages,
		"max_tokens":  b.MaxTokens,
		"temperature": b.Temperature,
		"stream":      true,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequest(http.MethodPost, b.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if b.EnvKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+os.Getenv(b.EnvKey))
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode != http.StatusOK {
		httpResp.Body.Close()
		return nil, fmt.Errorf("backend %s: %d", b.ID, httpResp.StatusCode)
	}
	ch := make(chan string, 8)
	go func() {
		defer httpResp.Body.Close()
		defer close(ch)
		sc := bufio.NewScanner(httpResp.Body)
		sc.Buffer(nil, 64*1024)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}
			var evt struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(data), &evt) != nil {
				continue
			}
			if len(evt.Choices) > 0 && evt.Choices[0].Delta.Content != "" {
				ch <- evt.Choices[0].Delta.Content
			}
		}
	}()
	return ch, nil
}
