// Package engine provides TF-IDF style skill routing.
package engine

import (
	"encoding/json"
	"math"
	"os"
	"regexp"
	"strings"
	"sync"
)

// SkillEntry declares a skill's keywords and priority.
type SkillEntry struct {
	ID          string   `json:"id"`
	Keywords    []string `json:"keywords"`
	Description string   `json:"description"`
	Priority    int      `json:"priority"` // higher = preferred when scores tie
}

// SkillRouterConfig holds router config.
type SkillRouterConfig struct {
	Skills   []SkillEntry `json:"skills"`
	ChainSep []string     `json:"chain_sep"` // e.g. ["然后","接着","再","and then"]
}

// SkillRouterImpl matches user text to skills via TF-IDF style scoring.
type SkillRouterImpl struct {
	mu       sync.RWMutex
	entries  []SkillEntry
	chainSep []*regexp.Regexp
	idf      map[string]float64
}

// NewSkillRouter creates a router with default skills.
func NewSkillRouter() *SkillRouterImpl {
	sr := &SkillRouterImpl{
		entries: defaultSkillEntries(),
		chainSep: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\s*然后\s*`),
			regexp.MustCompile(`(?i)\s*接着\s*`),
			regexp.MustCompile(`(?i)\s*再\s+`),
			regexp.MustCompile(`(?i)\s+and\s+then\s+`),
		},
		idf: make(map[string]float64),
	}
	sr.computeIDF()
	return sr
}

// NewSkillRouterFromConfig loads from JSON path.
func NewSkillRouterFromConfig(path string) *SkillRouterImpl {
	sr := NewSkillRouter()
	data, err := os.ReadFile(path)
	if err != nil {
		return sr
	}
	var cfg SkillRouterConfig
	if json.Unmarshal(data, &cfg) != nil {
		return sr
	}
	if len(cfg.Skills) > 0 {
		sr.mu.Lock()
		sr.entries = cfg.Skills
		sr.computeIDF()
		sr.mu.Unlock()
	}
	if len(cfg.ChainSep) > 0 {
		sr.mu.Lock()
		sr.chainSep = nil
		for _, s := range cfg.ChainSep {
			if re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(s)); err == nil {
				sr.chainSep = append(sr.chainSep, re)
			}
		}
		sr.mu.Unlock()
	}
	return sr
}

func defaultSkillEntries() []SkillEntry {
	return []SkillEntry{
		{ID: "web_search", Keywords: []string{"搜索", "查找", "查", "search", "find", "google", "百度"}, Description: "Web search", Priority: 80},
		{ID: "doc_perceiver", Keywords: []string{"文档", "文件", "解析", "摘要", "总结", "doc", "file", "parse", "summary", "read"}, Description: "Document parsing", Priority: 70},
		{ID: "tts", Keywords: []string{"朗读", "语音", "tts", "speak", "read aloud"}, Description: "Text to speech", Priority: 60},
		{ID: "openclaw_proxy", Keywords: []string{"openclaw", "代理", "proxy"}, Description: "OpenClaw proxy", Priority: 50},
		{ID: "mimicclaw_adapter", Keywords: []string{"mimicclaw", "mimic"}, Description: "MimicClaw", Priority: 40},
		{ID: "nanoclaw_adapter", Keywords: []string{"nanoclaw", "nano"}, Description: "NanoClaw", Priority: 40},
		{ID: "picoclaw_adapter", Keywords: []string{"picoclaw", "pico"}, Description: "PicoClaw", Priority: 40},
	}
}

func (sr *SkillRouterImpl) computeIDF() {
	docFreq := make(map[string]int)
	for _, e := range sr.entries {
		seen := make(map[string]bool)
		for _, kw := range e.Keywords {
			k := strings.ToLower(strings.TrimSpace(kw))
			if k == "" {
				continue
			}
			if !seen[k] {
				seen[k] = true
				docFreq[k]++
			}
		}
	}
	n := float64(len(sr.entries))
	if n < 1 {
		n = 1
	}
	sr.idf = make(map[string]float64)
	sr.idf = make(map[string]float64)
	for k, df := range docFreq {
		sr.idf[k] = math.Log(n/(float64(df)+0.5) + 1)
	}
}

// Route returns ordered skill IDs for the text. Supports chaining.
func (sr *SkillRouterImpl) Route(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	sr.mu.RLock()
	entries := sr.entries
	chainSep := sr.chainSep
	idf := sr.idf
	sr.mu.RUnlock()

	parts := sr.splitChain(text, chainSep)
	var all []string
	seen := make(map[string]bool)
	for _, part := range parts {
		ids := sr.routeOne(part, entries, idf)
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				all = append(all, id)
			}
		}
	}
	return all
}

func (sr *SkillRouterImpl) splitChain(text string, seps []*regexp.Regexp) []string {
	for _, re := range seps {
		if re.MatchString(text) {
			parts := re.Split(text, -1)
			var out []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					out = append(out, p)
				}
			}
			if len(out) > 1 {
				return out
			}
		}
	}
	return []string{text}
}

func (sr *SkillRouterImpl) routeOne(text string, entries []SkillEntry, idf map[string]float64) []string {
	lower := strings.ToLower(text)
	tokens := tokenize(lower)

	tf := make(map[string]float64)
	for _, t := range tokens {
		tf[t]++
	}
	for k := range tf {
		tf[k] = 1 + math.Log(tf[k]+1)
	}

	type scored struct {
		id    string
		score float64
		pri   int
	}
	var scoredList []scored
	for _, e := range entries {
		score := 0.0
		for _, kw := range e.Keywords {
			kw = strings.ToLower(kw)
			if strings.Contains(lower, kw) {
				tfVal := tf[kw]
				if tfVal == 0 {
					tfVal = 1
				}
				idfVal := idf[kw]
				if idfVal == 0 {
					idfVal = 1
				}
				score += tfVal * idfVal
				if strings.HasPrefix(lower, kw) || strings.HasSuffix(lower, kw) {
					score += 0.5
				}
			}
		}
		if score > 0 {
			scoredList = append(scoredList, scored{e.ID, score, e.Priority})
		}
	}
	if len(scoredList) == 0 {
		return nil
	}
	for i := 0; i < len(scoredList)-1; i++ {
		for j := i + 1; j < len(scoredList); j++ {
			if scoredList[j].score > scoredList[i].score ||
				(scoredList[j].score == scoredList[i].score && scoredList[j].pri > scoredList[i].pri) {
				scoredList[i], scoredList[j] = scoredList[j], scoredList[i]
			}
		}
	}
	out := make([]string, 0, len(scoredList))
	for _, s := range scoredList {
		out = append(out, s.id)
	}
	return out
}

func tokenize(s string) []string {
	re := regexp.MustCompile(`[\p{L}\p{N}]+|[^\s]+`)
	matches := re.FindAllString(s, -1)
	var out []string
	for _, m := range matches {
		m = strings.TrimSpace(m)
		if m != "" {
			out = append(out, strings.ToLower(m))
		}
	}
	return out
}

// AddSkill adds or updates a skill entry.
func (sr *SkillRouterImpl) AddSkill(e SkillEntry) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	found := false
	for i := range sr.entries {
		if sr.entries[i].ID == e.ID {
			sr.entries[i] = e
			found = true
			break
		}
	}
	if !found {
		sr.entries = append(sr.entries, e)
	}
	sr.computeIDF()
}
