package mimicclaw_adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"harmonclaw/governor"
	"harmonclaw/skills"
)

func init() {
	skills.Register(&Adapter{})
}

type Adapter struct{}

func (a *Adapter) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "mimicclaw_proxy", Version: "0.1.0", Core: "architect"}
}

func (a *Adapter) Execute(input skills.SkillInput) skills.SkillOutput {
	ctx := context.Background()
	return skills.RunSandboxed(ctx, input.TraceID, func() skills.SkillOutput {
		return a.doExecute(input)
	})
}

func (a *Adapter) doExecute(input skills.SkillInput) skills.SkillOutput {
	start := time.Now()
	apiURL := os.Getenv("MIMICCLAW_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:9001/execute"
	}

	body, _ := json.Marshal(map[string]any{
		"query":     input.Text,
		"args":      input.Args,
		"trace":     input.TraceID,
		"action_id": input.TraceID,
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	client := governor.SecureClient()
	resp, err := client.Do(req)
	if err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: err.Error()}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: err.Error()}
	}

	if resp.StatusCode != http.StatusOK {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: fmt.Sprintf("upstream %d: %s", resp.StatusCode, data)}
	}

	out := skills.SkillOutput{
		TraceID: input.TraceID,
		Status:  "ok",
		Data:    data,
	}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(data)
	return out
}
