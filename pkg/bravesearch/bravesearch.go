// Package bravesearch calls Brave Search API (no proxy); callers supply SecureClient and API key from env/config.
package bravesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const defaultEndpoint = "https://api.search.brave.com/res/v1/web/search"

func searchEndpoint() string {
	if v := strings.TrimSpace(os.Getenv("HC_BRAVE_API_BASE")); v != "" {
		return strings.TrimSuffix(v, "/") + "/res/v1/web/search"
	}
	return defaultEndpoint
}

// NormalizedItem matches web_search skill / inject JSON shape (title, url, snippet).
type NormalizedItem struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type apiEnvelope struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// Search performs a Brave web search. searchLang may be empty (Brave default).
func Search(ctx context.Context, httpClient *http.Client, apiKey, query string, count int, searchLang string) ([]NormalizedItem, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("bravesearch: api key is empty")
	}
	if count <= 0 {
		count = 5
	}
	u, err := url.Parse(searchEndpoint())
	if err != nil {
		return nil, fmt.Errorf("bravesearch: parse endpoint: %w", err)
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("count", fmt.Sprintf("%d", count))
	if searchLang != "" {
		q.Set("search_lang", searchLang)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("bravesearch: create request: %w", err)
	}
	req.Header.Set("X-Subscription-Token", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bravesearch: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bravesearch: status %d: %s", resp.StatusCode, string(body))
	}

	var env apiEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("bravesearch: decode: %w", err)
	}

	out := make([]NormalizedItem, 0, len(env.Web.Results))
	for _, r := range env.Web.Results {
		out = append(out, NormalizedItem{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		})
	}
	return out, nil
}

// ToJSON marshals normalized hits for skill / inject consumers.
func ToJSON(items []NormalizedItem) ([]byte, error) {
	return json.Marshal(items)
}
