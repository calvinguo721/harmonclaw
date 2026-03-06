package picoclaw_adapter

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

const (
	picoTimeout   = 5 * time.Second
	picoMaxBytes  = 1024
)

func init() {
	skills.Register(&Adapter{})
}

type Adapter struct{}

func (a *Adapter) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "picoclaw_proxy", Version: "0.1.0", Core: "architect"}
}

func (a *Adapter) Execute(input skills.SkillInput) skills.SkillOutput {
	ctx := context.Background()
	return skills.RunSandboxedWithTimeout(ctx, input.TraceID, picoTimeout, func() skills.SkillOutput {
		return a.doExecute(input)
	})
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func (a *Adapter) doExecute(input skills.SkillInput) skills.SkillOutput {
	start := time.Now()
	text := truncate(input.Text, picoMaxBytes)

	apiURL := os.Getenv("PICOCLAW_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:9003/invoke"
	}

	body, _ := json.Marshal(map[string]any{
		"query": text,
		"args":  input.Args,
		"trace": input.TraceID,
	})
	if len(body) > picoMaxBytes {
		body = body[:picoMaxBytes]
	}
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

	data, err := io.ReadAll(io.LimitReader(resp.Body, picoMaxBytes))
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
