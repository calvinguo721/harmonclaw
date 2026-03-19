// Package openclaw_adapter proxies OpenClaw v3.13+ API via POST /tools/invoke with Bearer auth.
package openclaw_adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"harmonclaw/governor"
	"harmonclaw/skills"
)

func init() {
	skills.Register(&Adapter{})
}

const (
	defaultEndpoint   = "http://localhost:3000"
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 3
	defaultMaxConcur  = 4
)

var openclawClient = governor.SecureClient()
var ocSem chan struct{}
var ocSemOnce sync.Once

type ocConfig struct {
	TimeoutSec    int `json:"timeout_sec"`
	MaxRetries    int `json:"max_retries"`
	MaxConcurrent int `json:"max_concurrent"`
	RetryBaseMs   int `json:"retry_base_ms"`
}

func loadOCConfig() ocConfig {
	cfg := ocConfig{
		TimeoutSec:    30,
		MaxRetries:    defaultMaxRetries,
		MaxConcurrent: defaultMaxConcur,
		RetryBaseMs:   500,
	}
	paths := []string{"configs/openclaw.json"}
	if wd, _ := os.Getwd(); wd != "" {
		paths = append(paths, filepath.Join(wd, "configs/openclaw.json"))
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		json.Unmarshal(data, &cfg)
		if cfg.TimeoutSec <= 0 {
			cfg.TimeoutSec = 30
		}
		if cfg.MaxRetries < 1 {
			cfg.MaxRetries = 1
		}
		break
	}
	return cfg
}

func initOCSem(n int) {
	ocSemOnce.Do(func() {
		if n <= 0 {
			n = defaultMaxConcur
		}
		ocSem = make(chan struct{}, n)
	})
}

func acquireOC() bool {
	cfg := loadOCConfig()
	initOCSem(cfg.MaxConcurrent)
	select {
	case ocSem <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseOC() {
	select {
	case <-ocSem:
	default:
	}
}

// Adapter implements skills.Skill for OpenClaw v3.13+ API.
type Adapter struct{}

// GetIdentity returns the skill identity.
func (a *Adapter) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "openclaw_proxy", Version: "0.4.0", Core: "architect"}
}

// Execute runs the OpenClaw tool via POST /tools/invoke.
func (a *Adapter) Execute(input skills.SkillInput) skills.SkillOutput {
	cfg := loadOCConfig()
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return skills.RunSandboxedWithTimeout(ctx, input.TraceID, timeout, func() skills.SkillOutput {
		return a.doExecute(input)
	})
}

func getEndpoint() string {
	if u := strings.TrimSpace(os.Getenv("HC_OPENCLAW_ENDPOINT")); u != "" {
		return u
	}
	return defaultEndpoint
}

func getToken() string {
	return strings.TrimSpace(os.Getenv("OPENCLAW_API_TOKEN"))
}

// OpenClawRequest is the v3.13+ request format for /tools/invoke.
type OpenClawRequest struct {
	Tool       string      `json:"tool"`
	Action     string      `json:"action"`
	Args       interface{} `json:"args"`
	SessionKey string      `json:"sessionKey"`
}

func (a *Adapter) doExecute(input skills.SkillInput) skills.SkillOutput {
	start := time.Now()
	if input.Args != nil && input.Args["sovereignty"] == "shadow" {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "offline mode"}
	}

	if !acquireOC() {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "openclaw proxy concurrency limit exceeded"}
	}
	defer releaseOC()

	endpoint := getEndpoint()
	args := make(map[string]interface{})
	if input.Args != nil {
		for k, v := range input.Args {
			args[k] = v
		}
	}
	args["query"] = input.Text
	args["text"] = input.Text

	ocReq := OpenClawRequest{
		Tool:       "openclaw",
		Action:     "invoke",
		Args:       args,
		SessionKey: input.TraceID,
	}
	body, _ := json.Marshal(ocReq)

	cfg := loadOCConfig()
	var out skills.SkillOutput
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	for i := 0; i < cfg.MaxRetries; i++ {
		out = a.doRequest(input.TraceID, endpoint, body, timeout, start)
		if out.Status == "ok" {
			return out
		}
		if i < cfg.MaxRetries-1 {
			backoff := time.Duration(cfg.RetryBaseMs*(1<<uint(i))) * time.Millisecond
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
			time.Sleep(backoff)
		}
	}
	return out
}

func openClawToHC(oc []byte) ([]byte, error) {
	var raw map[string]any
	if json.Unmarshal(oc, &raw) != nil {
		return oc, nil
	}
	if r, ok := raw["result"]; ok {
		return json.Marshal(r)
	}
	return oc, nil
}

func (a *Adapter) doRequest(traceID, endpoint string, body []byte, timeout time.Duration, start time.Time) skills.SkillOutput {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	url := strings.TrimSuffix(endpoint, "/") + "/tools/invoke"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return skills.SkillOutput{TraceID: traceID, Status: "error", Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if token := getToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := openclawClient.Do(req)
	if err != nil {
		return skills.SkillOutput{TraceID: traceID, Status: "error", Error: err.Error()}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return skills.SkillOutput{TraceID: traceID, Status: "error", Error: err.Error()}
	}

	if resp.StatusCode != http.StatusOK {
		return skills.SkillOutput{
			TraceID: traceID,
			Status:  "error",
			Error:   fmt.Sprintf("openclaw %d: %s", resp.StatusCode, data),
		}
	}

	converted, _ := openClawToHC(data)
	out := skills.SkillOutput{TraceID: traceID, Status: "ok", Data: converted}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(converted)
	return out
}
