package nanoclaw_adapter

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
	return skills.SkillIdentity{ID: "nanoclaw_proxy", Version: "0.1.0", Core: "architect"}
}

func (a *Adapter) Execute(input skills.SkillInput) skills.SkillOutput {
	ctx := context.Background()
	timeout := 30 * time.Second
	if input.Args != nil && input.Args["device_class"] == "constrained" {
		timeout = 10 * time.Second
	}
	return skills.RunSandboxedWithTimeout(ctx, input.TraceID, timeout, func() skills.SkillOutput {
		return a.doExecute(input)
	})
}

func (a *Adapter) doExecute(input skills.SkillInput) skills.SkillOutput {
	start := time.Now()
	apiURL := os.Getenv("NANOCLAW_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:9002/invoke"
	}

	body, _ := json.Marshal(map[string]any{
		"query": input.Text,
		"args":  input.Args,
		"trace": input.TraceID,
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
