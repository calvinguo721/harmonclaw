// Package web_search provides SearXNG search skill.
package web_search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"harmonclaw/governor"
	"harmonclaw/skills"
)

func init() {
	skills.Register(&Search{})
}

const searxURL = "http://localhost:8888/search"

type Search struct{}

func (s *Search) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "web_search", Version: "0.1.0", Core: "architect"}
}

func (s *Search) Execute(input skills.SkillInput) skills.SkillOutput {
	ctx := context.Background()
	return skills.RunSandboxed(ctx, input.TraceID, func() skills.SkillOutput {
		return s.doExecute(input)
	})
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"content"`
}

type searxResponse struct {
	Results []searchResult `json:"results"`
}

func (s *Search) doExecute(input skills.SkillInput) skills.SkillOutput {
	start := time.Now()
	if input.Args["sovereignty"] == "shadow" {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "offline mode"}
	}

	q := input.Text
	if q == "" && input.Args != nil {
		q = input.Args["q"]
	}
	if q == "" {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "query is empty"}
	}

	u := searxURL + "?q=" + url.QueryEscape(q) + "&format=json"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: err.Error()}
	}

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

	var sr searxResponse
	if err := json.Unmarshal(data, &sr); err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: fmt.Sprintf("parse: %v", err)}
	}

	top := 5
	if len(sr.Results) < top {
		top = len(sr.Results)
	}
	items := make([]map[string]string, 0, top)
	for i := 0; i < top; i++ {
		r := sr.Results[i]
		items = append(items, map[string]string{
			"title":   r.Title,
			"url":     r.URL,
			"snippet": r.Snippet,
		})
	}
	outData, _ := json.Marshal(items)

	out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: outData}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(outData)
	return out
}
