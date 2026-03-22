// Package skills — Brave Search skill and helpers for chat inject / tool execution.
package skills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"harmonclaw/configs"
	"harmonclaw/governor"
	"harmonclaw/pkg/bravesearch"
)

func init() {
	Register(&BraveSearchSkill{})
}

const braveSearchTimeout = 15 * time.Second

// braveSearchConfig is loaded from configs/brave_search.json (optional).
type braveSearchConfig struct {
	SearchLang   string `json:"search_lang"`
	DefaultCount int    `json:"default_count"`
}

var (
	braveCfgMu sync.RWMutex
	braveCfg   braveSearchConfig
)

func loadBraveSearchConfig() braveSearchConfig {
	path := strings.TrimSpace(os.Getenv("HC_BRAVE_CONFIG"))
	if path == "" {
		path = filepath.Join("configs", "brave_search.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return braveSearchConfig{SearchLang: "", DefaultCount: 10}
	}
	var c braveSearchConfig
	if json.Unmarshal(data, &c) != nil {
		return braveSearchConfig{DefaultCount: 10}
	}
	if c.DefaultCount <= 0 {
		c.DefaultCount = 10
	}
	return c
}

func getBraveSearchConfig() braveSearchConfig {
	braveCfgMu.Lock()
	defer braveCfgMu.Unlock()
	braveCfg = loadBraveSearchConfig()
	return braveCfg
}

func braveAPIKey() string {
	if k := strings.TrimSpace(os.Getenv("BRAVE_API_KEY")); k != "" {
		return k
	}
	if c := configs.Get(); c != nil {
		return strings.TrimSpace(c.BraveAPIKey)
	}
	return ""
}

// BraveSearchConfigured reports whether Brave API key is set (env or config).
func BraveSearchConfigured() bool {
	return braveAPIKey() != ""
}

// BraveSearchSkill implements H-Skill for direct Brave Search API calls.
type BraveSearchSkill struct{}

func (b *BraveSearchSkill) GetIdentity() SkillIdentity {
	return SkillIdentity{ID: "brave_search", Version: "0.1.0", Core: "architect"}
}

func (b *BraveSearchSkill) Execute(input SkillInput) SkillOutput {
	ctx, cancel := context.WithTimeout(context.Background(), braveSearchTimeout)
	defer cancel()
	return RunSandboxedWithTimeout(ctx, input.TraceID, braveSearchTimeout, func() SkillOutput {
		return b.doExecute(ctx, input)
	})
}

func (b *BraveSearchSkill) doExecute(ctx context.Context, input SkillInput) SkillOutput {
	if input.Args != nil && input.Args["sovereignty"] == "shadow" {
		return SkillOutput{TraceID: input.TraceID, Status: "error", Error: "offline mode"}
	}
	q := strings.TrimSpace(input.Text)
	if q == "" && input.Args != nil {
		q = strings.TrimSpace(input.Args["q"])
	}
	if q == "" {
		return SkillOutput{TraceID: input.TraceID, Status: "error", Error: "query is empty"}
	}
	key := braveAPIKey()
	if key == "" {
		return SkillOutput{TraceID: input.TraceID, Status: "error", Error: "BRAVE_API_KEY not configured"}
	}
	cfg := getBraveSearchConfig()
	count := cfg.DefaultCount
	if input.Args != nil {
		if v := strings.TrimSpace(input.Args["count"]); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				count = n
			}
		}
	}
	data, err := braveSearchNormalizedJSON(ctx, key, q, count, cfg.SearchLang)
	if err != nil {
		return SkillOutput{TraceID: input.TraceID, Status: "error", Error: err.Error()}
	}
	return SkillOutput{TraceID: input.TraceID, Status: "ok", Data: data}
}

// BraveSearchNormalizedJSON returns JSON array [{title,url,snippet},...] using Brave API and SecureClient.
func BraveSearchNormalizedJSON(ctx context.Context, query string, count int) ([]byte, error) {
	key := braveAPIKey()
	if key == "" {
		return nil, &configError{"BRAVE_API_KEY not configured"}
	}
	cfg := getBraveSearchConfig()
	if count <= 0 {
		count = cfg.DefaultCount
	}
	return braveSearchNormalizedJSON(ctx, key, query, count, cfg.SearchLang)
}

type configError struct{ msg string }

func (e *configError) Error() string { return e.msg }

func braveSearchNormalizedJSON(ctx context.Context, apiKey, query string, count int, searchLang string) ([]byte, error) {
	items, err := bravesearch.Search(ctx, governor.SecureClient(), apiKey, query, count, searchLang)
	if err != nil {
		return nil, err
	}
	return bravesearch.ToJSON(items)
}
