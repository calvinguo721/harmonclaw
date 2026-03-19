// Package web_search provides web search skill via API or DuckDuckGo HTML scraping.
package web_search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"harmonclaw/governor"
	"harmonclaw/skills"
)

func init() {
	skills.Register(&Search{})
}

const (
	defaultTimeout = 15 * time.Second
	defaultDDGURL  = "https://html.duckduckgo.com/html/"
	maxResults     = 10
)

var duckDuckGoURL = defaultDDGURL

var (
	reResultLink = regexp.MustCompile(`<a class="result__a" href="([^"]+)"[^>]*>([^<]+)</a>`)
	reSnippet    = regexp.MustCompile(`<a class="result__snippet"[^>]*>([^<]*)</a>`)
)

type Search struct{}

func (s *Search) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "web_search", Version: "0.2.0", Core: "architect"}
}

func (s *Search) Execute(input skills.SkillInput) skills.SkillOutput {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	return skills.RunSandboxedWithTimeout(ctx, input.TraceID, defaultTimeout, func() skills.SkillOutput {
		return s.doExecute(input)
	})
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func (s *Search) doExecute(input skills.SkillInput) skills.SkillOutput {
	start := time.Now()
	if input.Args != nil && input.Args["sovereignty"] == "shadow" {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "offline mode"}
	}

	q := input.Text
	if q == "" && input.Args != nil {
		q = input.Args["q"]
	}
	if q == "" {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "query is empty"}
	}
	q = strings.TrimSpace(q)
	cacheKey := "q:" + strings.ToLower(q)

	if cached, ok := cacheGet(cacheKey); ok {
		out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: cached}
		out.Metrics.Ms = time.Since(start).Milliseconds()
		out.Metrics.Bytes = len(cached)
		return out
	}

	if !acquireSearchSlot() {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "search concurrency limit exceeded"}
	}
	defer releaseSearchSlot()

	var out skills.SkillOutput
	apiURL := getSearchAPIURL()
	searxURL := getSearXNGURL()
	if apiURL != "" {
		out = s.searchViaAPI(input, q, apiURL, start)
	} else if searxURL != "" {
		out = s.searchViaSearXNG(input, q, searxURL, start)
	} else {
		out = s.searchViaDuckDuckGo(input, q, getDuckDuckGoURL(), start)
	}
	if out.Status == "ok" && len(out.Data) > 0 {
		cacheSet(cacheKey, out.Data)
	}
	return out
}

func getSearchAPIURL() string {
	if u := strings.TrimSpace(os.Getenv("HC_SEARCH_API")); u != "" {
		return u
	}
	return ""
}

func getSearXNGURL() string {
	if u := strings.TrimSpace(os.Getenv("HC_SEARCH_SEARXNG")); u != "" {
		return strings.TrimSuffix(u, "/")
	}
	return ""
}

func getDuckDuckGoURL() string {
	return duckDuckGoURL
}

func (s *Search) searchViaAPI(input skills.SkillInput, q, apiURL string, start time.Time) skills.SkillOutput {
	u := apiURL
	if !strings.Contains(apiURL, "?") {
		u = apiURL + "?q=" + url.QueryEscape(q) + "&format=json"
	} else {
		u = apiURL + "&q=" + url.QueryEscape(q)
	}
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

	var sr struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &sr); err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: fmt.Sprintf("parse: %v", err)}
	}

	top := maxResults
	if len(sr.Results) < top {
		top = len(sr.Results)
	}
	items := make([]searchResult, 0, top)
	for i := 0; i < top; i++ {
		r := sr.Results[i]
		items = append(items, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Snippet})
	}
	outData, _ := json.Marshal(items)

	out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: outData}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(outData)
	return out
}

func (s *Search) searchViaSearXNG(input skills.SkillInput, q string, baseURL string, start time.Time) skills.SkillOutput {
	u := baseURL + "/search?q=" + url.QueryEscape(q) + "&format=json"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: err.Error()}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; HarmonClaw/1.0)")

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

	var sr struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &sr); err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: fmt.Sprintf("searxng parse: %v", err)}
	}

	top := maxResults
	if len(sr.Results) < top {
		top = len(sr.Results)
	}
	items := make([]searchResult, 0, top)
	for i := 0; i < top; i++ {
		r := sr.Results[i]
		items = append(items, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	outData, _ := json.Marshal(items)

	out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: outData}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(outData)
	return out
}

func (s *Search) searchViaDuckDuckGo(input skills.SkillInput, q string, baseURL string, start time.Time) skills.SkillOutput {
	form := url.Values{}
	form.Set("q", q)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL, strings.NewReader(form.Encode()))
	if err != nil {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; HarmonClaw/1.0)")

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

	html := string(data)
	links := reResultLink.FindAllStringSubmatch(html, maxResults)
	snippets := reSnippet.FindAllStringSubmatch(html, maxResults)

	items := make([]searchResult, 0, len(links))
	for i, m := range links {
		if len(m) >= 3 {
			title := strings.TrimSpace(htmlUnescape(m[2]))
			link := strings.TrimSpace(m[1])
			snippet := ""
			if i < len(snippets) && len(snippets[i]) >= 2 {
				snippet = strings.TrimSpace(htmlUnescape(snippets[i][1]))
			}
			if link != "" && title != "" {
				items = append(items, searchResult{Title: title, URL: link, Snippet: snippet})
			}
		}
	}
	outData, _ := json.Marshal(items)

	out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: outData}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(outData)
	return out
}

func htmlUnescape(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	return s
}
