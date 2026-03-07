// Package openclaw_adapter proxies OpenClaw API with format conversion and retry.
package openclaw_adapter

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

const defaultEndpoint = "http://localhost:3000"

var openclawClient = governor.SecureClient()

type Adapter struct{}

func (a *Adapter) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "openclaw_proxy", Version: "0.2.0", Core: "architect"}
}

func (a *Adapter) Execute(input skills.SkillInput) skills.SkillOutput {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return skills.RunSandboxedWithTimeout(ctx, input.TraceID, 30*time.Second, func() skills.SkillOutput {
		return a.doExecute(input)
	})
}

func getEndpoint() string {
	if u := strings.TrimSpace(os.Getenv("HC_OPENCLAW_ENDPOINT")); u != "" {
		return u
	}
	return defaultEndpoint
}

func (a *Adapter) doExecute(input skills.SkillInput) skills.SkillOutput {
	start := time.Now()
	endpoint := getEndpoint()

	hcReq := map[string]any{
		"text":   input.Text,
		"args":   input.Args,
		"trace":  input.TraceID,
	}
	ocReq := hcToOpenClaw(hcReq)
	body, _ := json.Marshal(ocReq)

	out := a.doRequest(input.TraceID, endpoint, body, start)
	if out.Status == "ok" {
		return out
	}
	out = a.doRequest(input.TraceID, endpoint, body, start)
	return out
}

func hcToOpenClaw(hc map[string]any) map[string]any {
	return map[string]any{
		"query": hc["text"],
		"args":  hc["args"],
		"trace": hc["trace"],
	}
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

func (a *Adapter) doRequest(traceID, endpoint string, body []byte, start time.Time) skills.SkillOutput {
	url := strings.TrimSuffix(endpoint, "/") + "/invoke"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return skills.SkillOutput{TraceID: traceID, Status: "error", Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

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
