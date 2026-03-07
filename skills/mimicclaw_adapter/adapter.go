// Package mimicclaw_adapter proxies MimicClaw imitation learning API.
package mimicclaw_adapter

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

const defaultEndpoint = "http://localhost:9001"

type proxyCfg struct {
	TimeoutSec   int `json:"timeout_sec"`
	MaxRetries   int `json:"max_retries"`
	MaxConcurrent int `json:"max_concurrent"`
}

var mimicSem chan struct{}
var mimicSemOnce sync.Once

func loadMimicConfig() proxyCfg {
	cfg := proxyCfg{TimeoutSec: 30, MaxRetries: 2, MaxConcurrent: 2}
	paths := []string{"configs/proxy_claw.json"}
	if wd, _ := os.Getwd(); wd != "" {
		paths = append(paths, filepath.Join(wd, "configs/proxy_claw.json"))
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var raw map[string]proxyCfg
		if json.Unmarshal(data, &raw) == nil && raw["mimicclaw"].TimeoutSec > 0 {
			cfg = raw["mimicclaw"]
			break
		}
	}
	return cfg
}

func mimicAcquire() bool {
	cfg := loadMimicConfig()
	mimicSemOnce.Do(func() {
		n := cfg.MaxConcurrent
		if n <= 0 {
			n = 2
		}
		mimicSem = make(chan struct{}, n)
	})
	select {
	case mimicSem <- struct{}{}:
		return true
	default:
		return false
	}
}

func mimicRelease() {
	select {
	case <-mimicSem:
	default:
	}
}

type Adapter struct{}

func (a *Adapter) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "mimicclaw_proxy", Version: "0.3.0", Core: "architect"}
}

func (a *Adapter) Execute(input skills.SkillInput) skills.SkillOutput {
	cfg := loadMimicConfig()
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return skills.RunSandboxedWithTimeout(ctx, input.TraceID, timeout, func() skills.SkillOutput {
		return a.doExecute(input)
	})
}

func (a *Adapter) doExecute(input skills.SkillInput) skills.SkillOutput {
	start := time.Now()
	if input.Args != nil && input.Args["sovereignty"] == "shadow" {
		return a.degraded(input.TraceID, "offline mode", start)
	}
	if !mimicAcquire() {
		return a.degraded(input.TraceID, "concurrency limit", start)
	}
	defer mimicRelease()

	apiURL := strings.TrimSpace(os.Getenv("HC_MIMICCLAW_ENDPOINT"))
	if apiURL == "" {
		apiURL = defaultEndpoint
	}

	body, _ := json.Marshal(map[string]any{
		"query": input.Text,
		"args":  input.Args,
		"trace": input.TraceID,
	})
	cfg := loadMimicConfig()
	var out skills.SkillOutput
	for i := 0; i < cfg.MaxRetries; i++ {
		out = a.doRequest(input.TraceID, apiURL, body, start)
		if out.Status == "ok" {
			return out
		}
		if i < cfg.MaxRetries-1 {
			time.Sleep(time.Duration(200*(1<<uint(i))) * time.Millisecond)
		}
	}
	return a.degraded(input.TraceID, out.Error, start)
}

func (a *Adapter) doRequest(traceID, apiURL string, body []byte, start time.Time) skills.SkillOutput {
	cfg := loadMimicConfig()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSec)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/execute", bytes.NewReader(body))
	if err != nil {
		return skills.SkillOutput{TraceID: traceID, Status: "error", Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := governor.SecureClient().Do(req)
	if err != nil {
		return skills.SkillOutput{TraceID: traceID, Status: "error", Error: err.Error()}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return skills.SkillOutput{TraceID: traceID, Status: "error", Error: err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		return skills.SkillOutput{TraceID: traceID, Status: "error", Error: fmt.Sprintf("upstream %d", resp.StatusCode)}
	}
	out := skills.SkillOutput{TraceID: traceID, Status: "ok", Data: data}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(data)
	return out
}

func (a *Adapter) degraded(traceID, reason string, start time.Time) skills.SkillOutput {
	data, _ := json.Marshal(map[string]any{
		"degraded": true,
		"reason":   reason,
		"message":  "MimicClaw unavailable, using fallback",
	})
	out := skills.SkillOutput{TraceID: traceID, Status: "ok", Data: data}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(data)
	return out
}
