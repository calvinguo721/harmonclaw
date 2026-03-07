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
	"strings"
	"time"

	"harmonclaw/governor"
	"harmonclaw/skills"
)

func init() {
	skills.Register(&Adapter{})
}

const (
	defaultEndpoint = "http://localhost:9003"
	picoTimeout     = 5 * time.Second
	picoMaxBytes    = 1024
)

type Adapter struct{}

func (a *Adapter) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "picoclaw_proxy", Version: "0.2.0", Core: "architect"}
}

func (a *Adapter) Execute(input skills.SkillInput) skills.SkillOutput {
	ctx, cancel := context.WithTimeout(context.Background(), picoTimeout)
	defer cancel()
	return skills.RunSandboxedWithTimeout(ctx, input.TraceID, picoTimeout, func() skills.SkillOutput {
		return a.doExecute(input)
	})
}

func (a *Adapter) doExecute(input skills.SkillInput) skills.SkillOutput {
	start := time.Now()
	apiURL := strings.TrimSpace(os.Getenv("HC_PICOCLAW_ENDPOINT"))
	if apiURL == "" {
		apiURL = defaultEndpoint
	}

	text := input.Text
	if len(text) > picoMaxBytes {
		text = text[:picoMaxBytes]
	}
	body, _ := json.Marshal(map[string]any{"query": text, "args": input.Args, "trace": input.TraceID})
	if len(body) > picoMaxBytes {
		body = body[:picoMaxBytes]
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, apiURL+"/invoke", bytes.NewReader(body))
	if err != nil {
		return a.degraded(input.TraceID, err.Error(), start)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := governor.SecureClient().Do(req)
	if err != nil {
		return a.degraded(input.TraceID, err.Error(), start)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, picoMaxBytes))
	if err != nil {
		return a.degraded(input.TraceID, err.Error(), start)
	}
	if resp.StatusCode != http.StatusOK {
		return a.degraded(input.TraceID, fmt.Sprintf("upstream %d", resp.StatusCode), start)
	}

	out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: data}
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
