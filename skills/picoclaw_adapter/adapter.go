// Package picoclaw_adapter proxies PicoClaw for MCU-level devices.
package picoclaw_adapter

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
	defaultEndpoint = "http://localhost:9003"
	picoMaxBytes    = 1024
)

type picoCfg struct {
	TimeoutSec    int `json:"timeout_sec"`
	MaxRetries    int `json:"max_retries"`
	MaxConcurrent int `json:"max_concurrent"`
	MaxBytes      int `json:"max_bytes"`
}

var picoSem chan struct{}
var picoSemOnce sync.Once

func loadPicoConfig() picoCfg {
	cfg := picoCfg{TimeoutSec: 5, MaxRetries: 1, MaxConcurrent: 1, MaxBytes: picoMaxBytes}
	paths := []string{"configs/proxy_claw.json"}
	if wd, _ := os.Getwd(); wd != "" {
		paths = append(paths, filepath.Join(wd, "configs/proxy_claw.json"))
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var raw map[string]picoCfg
		if json.Unmarshal(data, &raw) == nil && raw["picoclaw"].TimeoutSec > 0 {
			cfg = raw["picoclaw"]
			if cfg.MaxBytes <= 0 {
				cfg.MaxBytes = picoMaxBytes
			}
			break
		}
	}
	return cfg
}

func picoAcquire() bool {
	cfg := loadPicoConfig()
	picoSemOnce.Do(func() {
		n := cfg.MaxConcurrent
		if n <= 0 {
			n = 1
		}
		picoSem = make(chan struct{}, n)
	})
	select {
	case picoSem <- struct{}{}:
		return true
	default:
		return false
	}
}

func picoRelease() {
	select {
	case <-picoSem:
	default:
	}
}

type Adapter struct{}

func (a *Adapter) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "picoclaw_proxy", Version: "0.3.0", Core: "architect"}
}

func (a *Adapter) Execute(input skills.SkillInput) skills.SkillOutput {
	cfg := loadPicoConfig()
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
	if !picoAcquire() {
		return a.degraded(input.TraceID, "concurrency limit", start)
	}
	defer picoRelease()

	cfg := loadPicoConfig()
	apiURL := strings.TrimSpace(os.Getenv("HC_PICOCLAW_ENDPOINT"))
	if apiURL == "" {
		apiURL = defaultEndpoint
	}

	text := input.Text
	if len(text) > cfg.MaxBytes {
		text = text[:cfg.MaxBytes]
	}
	body, _ := json.Marshal(map[string]any{"query": text, "args": input.Args, "trace": input.TraceID})
	if len(body) > cfg.MaxBytes {
		body = body[:cfg.MaxBytes]
	}

	var out skills.SkillOutput
	for i := 0; i < cfg.MaxRetries; i++ {
		out = a.doRequest(input.TraceID, apiURL, body, cfg.MaxBytes, start)
		if out.Status == "ok" {
			return out
		}
		if i < cfg.MaxRetries-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	return a.degraded(input.TraceID, out.Error, start)
}

func (a *Adapter) doRequest(traceID, apiURL string, body []byte, maxBytes int, start time.Time) skills.SkillOutput {
	cfg := loadPicoConfig()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSec)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/invoke", bytes.NewReader(body))
	if err != nil {
		return skills.SkillOutput{TraceID: traceID, Status: "error", Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := governor.SecureClient().Do(req)
	if err != nil {
		return skills.SkillOutput{TraceID: traceID, Status: "error", Error: err.Error()}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)))
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
	data, _ := json.Marshal(map[string]any{"degraded": true, "reason": reason, "message": "PicoClaw unavailable"})
	out := skills.SkillOutput{TraceID: traceID, Status: "ok", Data: data}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(data)
	return out
}
