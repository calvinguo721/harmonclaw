// Package engine provides skill routing from user intent (OpenClaw-style).
package engine

import (
	"regexp"
	"strings"

	"harmonclaw/skills"
)

// SkillRouter matches user intent to skills, supports multi-skill combo.
type SkillRouter struct {
	keywords map[string][]string
	reCache  map[string]*regexp.Regexp
}

// NewSkillRouter creates a router with default keyword mappings.
func NewSkillRouter() *SkillRouter {
	sr := &SkillRouter{
		keywords: map[string][]string{
			"doc_perceiver":    {"文档", "文件", "解析", "摘要", "doc", "file", "parse", "summary", "read"},
			"web_search":       {"搜索", "查", "search", "find", "google", "百度"},
			"tts":              {"朗读", "语音", "tts", "speak", "read aloud"},
			"openclaw_proxy":   {"openclaw", "代理", "proxy"},
		},
		reCache: make(map[string]*regexp.Regexp),
	}
	return sr
}

// Route returns ordered skill IDs for the user text. Empty if no match.
func (sr *SkillRouter) Route(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return nil
	}
	var scored []struct {
		id    string
		score int
	}
	for skillID, kws := range sr.keywords {
		if _, ok := skills.Registry[skillID]; !ok {
			continue
		}
		score := sr.matchScore(text, kws)
		if score > 0 {
			scored = append(scored, struct {
				id    string
				score int
			}{skillID, score})
		}
	}
	if len(scored) == 0 {
		return nil
	}
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	out := make([]string, 0, len(scored))
	seen := make(map[string]bool)
	for _, s := range scored {
		if !seen[s.id] {
			out = append(out, s.id)
			seen[s.id] = true
		}
	}
	return out
}

func (sr *SkillRouter) matchScore(text string, kws []string) int {
	score := 0
	for _, kw := range kws {
		kw = strings.ToLower(kw)
		if strings.Contains(text, kw) {
			score += 2
		}
		if strings.HasPrefix(text, kw) || strings.HasSuffix(text, kw) {
			score += 1
		}
	}
	return score
}

// RouteWithFallback returns skill IDs or fallbackID if no match.
func (sr *SkillRouter) RouteWithFallback(text, fallbackID string) []string {
	ids := sr.Route(text)
	if len(ids) == 0 && fallbackID != "" {
		if _, ok := skills.Registry[fallbackID]; ok {
			return []string{fallbackID}
		}
	}
	return ids
}

// AddKeywords adds or overrides keywords for a skill.
func (sr *SkillRouter) AddKeywords(skillID string, kws []string) {
	sr.keywords[skillID] = kws
}

// MatchRegex matches text against a regex pattern.
func (sr *SkillRouter) MatchRegex(text, pattern string) bool {
	re, ok := sr.reCache[pattern]
	if !ok {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return false
		}
		sr.reCache[pattern] = re
	}
	return re.MatchString(text)
}
