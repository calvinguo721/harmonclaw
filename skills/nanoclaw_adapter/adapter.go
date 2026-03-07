// Package nanoclaw_adapter proxies NanoClaw for resource-constrained devices.
package nanoclaw_adapter

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

const defaultEndpoint = "http://localhost:9002"

type Adapter struct{}

func (a *Adapter) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "nanoclaw_proxy", Version: "0.2.0", Core: "architect"}
}

func (a *Adapter) Execute(input skills.SkillInput) skills.SkillOutput {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return skills.RunSandboxedWithTimeout(ctx, input.TraceID, 30*time.Second, func() skills.SkillOutput {
		return a.doExecute(input)
	})
}

func (a *Adapter) doExecute(input skills.SkillInput) skills.SkillOutput {
	start := time.Now()
	apiURL := strings.TrimSpace(os.Getenv("HC_NANOCLAW_ENDPOINT"))
	if apiURL == "" {
		apiURL = defaultEndpoint
	}

	body, _ := json.Marshal(map[string]any{"query": input.Text, "args": input.Args, "trace": input.TraceID})
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

	data, err := io.ReadAll(resp.Body)
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
	data, _ := json.Marshal(map[string]any{"degraded": true, "reason": reason, "message": "NanoClaw unavailable"})
	out := skills.SkillOutput{TraceID: traceID, Status: "ok", Data: data}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(data)
	return out
}
