// Package engine provides intent recognition and conversation logic.
package engine

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"sync"
)

// Intent types.
const (
	IntentChat    = "chat"
	IntentSearch  = "search"
	IntentSkill   = "skill"
	IntentMemory  = "memory"
	IntentSystem  = "system"
)

// IntentResult holds recognized intent with confidence.
type IntentResult struct {
	Intent     string
	Confidence float64
	Extracted  string
}

// IntentConfig per intent.
type IntentConfig struct {
	Keywords []string `json:"keywords"`
	Patterns []string `json:"patterns"`
}

// IntentRecognizer recognizes user intent from text.
type IntentRecognizer struct {
	mu       sync.RWMutex
	rules    map[string]IntentConfig
	patterns map[string][]*regexp.Regexp
	last     map[string]string
}

// NewIntentRecognizer creates a recognizer. Loads from configPath if exists.
func NewIntentRecognizer(configPath string) *IntentRecognizer {
	r := &IntentRecognizer{
		rules:    make(map[string]IntentConfig),
		patterns: make(map[string][]*regexp.Regexp),
		last:     make(map[string]string),
	}
	if configPath == "" {
		configPath = "configs/intents.json"
	}
	data, err := os.ReadFile(configPath)
	if err == nil {
		var cfg map[string]IntentConfig
		if json.Unmarshal(data, &cfg) == nil {
			r.rules = cfg
		}
	}
	if len(r.rules) == 0 {
		r.rules = defaultIntentRules()
	}
	r.compilePatterns()
	return r
}

func defaultIntentRules() map[string]IntentConfig {
	return map[string]IntentConfig{
		IntentChat:   {Keywords: []string{"你好", "hello", "hi", "hey"}, Patterns: []string{}},
		IntentSearch: {Keywords: []string{"搜索", "查找", "search", "find"}, Patterns: []string{}},
		IntentSkill:  {Keywords: []string{"执行", "运行", "execute", "run"}, Patterns: []string{}},
		IntentMemory: {Keywords: []string{"记得", "之前", "recall", "remember"}, Patterns: []string{}},
		IntentSystem: {Keywords: []string{"状态", "健康", "status", "version"}, Patterns: []string{}},
	}
}

func (r *IntentRecognizer) compilePatterns() {
	for intent, cfg := range r.rules {
		var compiled []*regexp.Regexp
		for _, p := range cfg.Patterns {
			if re, err := regexp.Compile("(?i)" + p); err == nil {
				compiled = append(compiled, re)
			}
		}
		r.patterns[intent] = compiled
	}
}

// Recognize returns the best intent for text. sessionID for multi-turn context.
func (r *IntentRecognizer) Recognize(text, sessionID string) IntentResult {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	if text == "" {
		return IntentResult{Intent: IntentChat, Confidence: 0}
	}

	r.mu.RLock()
	lastIntent := r.last[sessionID]
	r.mu.RUnlock()

	scores := make(map[string]float64)

	for intent, cfg := range r.rules {
		score := 0.0

		for _, kw := range cfg.Keywords {
			kwLower := strings.ToLower(kw)
			if strings.Contains(lower, kwLower) {
				score += 0.3
			}
			if strings.HasPrefix(lower, kwLower) || strings.HasSuffix(lower, kwLower) {
				score += 0.1
			}
		}

		for _, re := range r.patterns[intent] {
			if re.MatchString(text) || re.MatchString(lower) {
				score += 0.5
			}
		}

		if score > 1 {
			score = 1
		}
		scores[intent] = score
	}

	if lastIntent != "" && (lower == "继续" || lower == "continue" || lower == "接着" || lower == "然后") {
		if s, ok := scores[lastIntent]; ok {
			scores[lastIntent] = s + 0.4
			if scores[lastIntent] > 1 {
				scores[lastIntent] = 1
			}
		} else {
			scores[lastIntent] = 0.8
		}
	}

	order := []string{IntentSkill, IntentSearch, IntentMemory, IntentSystem, IntentChat}
	best := IntentChat
	bestScore := 0.0
	for _, intent := range order {
		if score := scores[intent]; score > bestScore {
			bestScore = score
			best = intent
		}
	}

	if bestScore < 0.1 {
		best = IntentChat
		bestScore = 0.5
	}

	r.mu.Lock()
	r.last[sessionID] = best
	r.mu.Unlock()

	return IntentResult{Intent: best, Confidence: bestScore, Extracted: text}
}

// SetLastIntent sets last intent for session (for testing).
func (r *IntentRecognizer) SetLastIntent(sessionID, intent string) {
	r.mu.Lock()
	r.last[sessionID] = intent
	r.mu.Unlock()
}
