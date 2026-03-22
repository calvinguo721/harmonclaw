// Package butler injects web_search results into chat when the user asks for real-time / web info.
package butler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"harmonclaw/governor"
	"harmonclaw/llm"
	"harmonclaw/skills"
)

type searchHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// injectWebSearchContext runs web search for the last user turn when appropriate, and prepends a system message with results.
func injectWebSearchContext(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		log.Printf("[web_search_inject] skip: no messages")
		return msgs
	}
	lastIdx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			lastIdx = i
			break
		}
	}
	if lastIdx < 0 {
		log.Printf("[web_search_inject] skip: no user role message in context")
		return msgs
	}
	lastUserMsg := strings.TrimSpace(msgs[lastIdx].Content)
	log.Printf("[web_search_inject] checking last user message: %s", lastUserMsg)
	if lastUserMsg == "" {
		log.Printf("[web_search_inject] skip: empty last user message")
		return msgs
	}
	if !shouldTriggerWebSearch(lastUserMsg) {
		log.Printf("[web_search_inject] skip: trigger rules did not match")
		return msgs
	}
	query := extractSearchQuery(lastUserMsg)
	log.Printf("[web_search_inject] triggered! query=%s", query)

	var data []byte
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if skills.BraveSearchConfigured() {
		log.Printf("[web_search_inject] using Brave Search API (direct)")
		var err error
		data, err = skills.BraveSearchNormalizedJSON(ctx, query, 0)
		if err != nil {
			log.Printf("[web_search_inject] Brave direct failed, will try web_search skill: %v", err)
			data = nil
		}
	}

	if len(data) == 0 {
		sk, ok := skills.Registry["web_search"]
		if !ok {
			log.Printf("[web_search_inject] skip: web_search skill not registered and Brave unavailable")
			return msgs
		}
		log.Printf("[web_search_inject] calling web_search skill fallback")
		mode, _ := governor.GetSovereigntyMode()
		out := sk.Execute(skills.SkillInput{
			TraceID: fmt.Sprintf("chat-ws-%d", time.Now().UnixNano()),
			Text:    query,
			Args:    map[string]string{"sovereignty": mode},
		})
		if out.Status != "ok" {
			log.Printf("[web_search_inject] skill returned status=%s error=%q", out.Status, out.Error)
			return msgs
		}
		if len(out.Data) == 0 {
			log.Printf("[web_search_inject] skill returned empty data")
			return msgs
		}
		data = out.Data
		log.Printf("[web_search_inject] skill returned payload len=%d", len(data))
	}

	if len(data) == 0 {
		log.Printf("[web_search_inject] skip: empty search payload")
		return msgs
	}

	var parsed []searchHit
	if err := json.Unmarshal(data, &parsed); err != nil {
		log.Printf("[web_search_inject] json decode (normalized results) failed: %v", err)
		return msgs
	}
	log.Printf("[web_search_inject] parsed %d result items for prompt", len(parsed))

	formatted := formatSearchResultsForLLM(data)
	if formatted == "" {
		log.Printf("[web_search_inject] skip: formatSearchResultsForLLM empty (no usable snippets?)")
		return msgs
	}
	prefix := "The user asked for information that may require up-to-date web data. Use the following search results to answer; cite titles/URLs when relevant. If results are empty or irrelevant, say so.\n\n"
	injectedText := prefix + formatted
	log.Printf("[web_search_inject] injected %d chars into system prompt", len(injectedText))

	sys := llm.Message{Role: "system", Content: injectedText}
	outMsgs := make([]llm.Message, 0, len(msgs)+1)
	outMsgs = append(outMsgs, msgs[:lastIdx]...)
	outMsgs = append(outMsgs, sys)
	outMsgs = append(outMsgs, msgs[lastIdx:]...)
	return outMsgs
}

func shouldTriggerWebSearch(s string) bool {
	lower := strings.ToLower(s)
	if strings.Contains(lower, "search") || strings.Contains(s, "搜索") {
		return true
	}
	if strings.Contains(lower, "google ") || strings.Contains(lower, "bing ") {
		return true
	}
	if (strings.Contains(lower, "weather") || strings.Contains(s, "天气")) &&
		(strings.Contains(lower, "today") || strings.Contains(s, "今天") ||
			strings.Contains(lower, "current") || strings.Contains(s, "实时") ||
			strings.Contains(lower, " now") || strings.HasPrefix(lower, "now ")) {
		return true
	}
	if strings.Contains(s, "最新") && (strings.Contains(s, "新闻") || strings.Contains(s, "消息")) {
		return true
	}
	return false
}

func extractSearchQuery(s string) string {
	s = strings.TrimSpace(s)
	for _, p := range []string{"please search for ", "please search ", "search for ", "search "} {
		if rest, ok := stripPrefixFold(s, p); ok {
			return rest
		}
	}
	for _, p := range []string{"请搜索", "搜索"} {
		if strings.HasPrefix(s, p) {
			return strings.TrimSpace(s[len(p):])
		}
	}
	return s
}

func stripPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) {
		return s, false
	}
	if strings.EqualFold(s[:len(prefix)], prefix) {
		return strings.TrimSpace(s[len(prefix):]), true
	}
	return s, false
}

func formatSearchResultsForLLM(data []byte) string {
	var items []searchHit
	if err := json.Unmarshal(data, &items); err != nil || len(items) == 0 {
		return ""
	}
	var b strings.Builder
	for i, it := range items {
		if i >= 8 {
			break
		}
		b.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n\n", i+1, it.Title, it.URL, it.Snippet))
	}
	return strings.TrimSpace(b.String())
}
