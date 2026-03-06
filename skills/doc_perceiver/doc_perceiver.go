package doc_perceiver

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"harmonclaw/skills"
)

func init() {
	skills.Register(&Perceiver{})
}

var (
	reDate = regexp.MustCompile(`\d{4}[-/]\d{1,2}[-/]\d{1,2}`)
	reURL  = regexp.MustCompile(`https?://[^\s]+`)
	reNum  = regexp.MustCompile(`\b\d{2,}(?:\.\d+)?\b`)
)

type Perceiver struct{}

func (p *Perceiver) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "doc_perceiver", Version: "0.1.0", Core: "architect"}
}

func (p *Perceiver) Execute(input skills.SkillInput) skills.SkillOutput {
	start := time.Now()

	if input.Text == "" {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "input text is empty"}
	}

	result := docResult{
		Title:       extractTitle(input.Text),
		Sections:    extractSections(input.Text),
		Entities:    extractEntities(input.Text),
		ActionItems: extractActionItems(input.Text),
	}

	data, _ := json.Marshal(result)
	out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: data}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(data)
	out.Metrics.Tokens = len(input.Text) / 4
	return out
}

// --- output schema ---

type docResult struct {
	Title       string       `json:"title"`
	Sections    []section    `json:"sections"`
	Entities    []entity     `json:"entities"`
	ActionItems []actionItem `json:"action_items"`
}

type section struct {
	Heading string   `json:"heading"`
	Bullets []string `json:"bullets"`
}

type entity struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type actionItem struct {
	Text     string `json:"text"`
	Priority string `json:"priority"`
}

// --- extractors (all local-scope, GC-friendly) ---

func extractTitle(text string) string {
	for _, line := range strings.SplitN(text, "\n", 10) {
		t := strings.TrimSpace(line)
		if t != "" {
			return strings.TrimSpace(strings.TrimLeft(t, "#"))
		}
	}
	return "Untitled"
}

func extractSections(text string) []section {
	var secs []section
	cur := -1

	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#") {
			heading := strings.TrimSpace(strings.TrimLeft(t, "#"))
			secs = append(secs, section{Heading: heading})
			cur = len(secs) - 1
		} else if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") {
			bullet := strings.TrimSpace(t[2:])
			if cur >= 0 {
				secs[cur].Bullets = append(secs[cur].Bullets, bullet)
			} else {
				secs = append(secs, section{Heading: "General", Bullets: []string{bullet}})
				cur = len(secs) - 1
			}
		}
	}

	if len(secs) == 0 {
		preview := text
		if len(preview) > 200 {
			preview = preview[:200]
		}
		secs = []section{{Heading: "Content", Bullets: []string{strings.TrimSpace(preview)}}}
	}
	return secs
}

func extractEntities(text string) []entity {
	var ents []entity
	for _, m := range reDate.FindAllString(text, 20) {
		ents = append(ents, entity{Type: "date", Value: m})
	}
	for _, m := range reURL.FindAllString(text, 20) {
		ents = append(ents, entity{Type: "url", Value: m})
	}
	for _, m := range reNum.FindAllString(text, 20) {
		ents = append(ents, entity{Type: "number", Value: m})
	}
	return ents
}

func extractActionItems(text string) []actionItem {
	var items []actionItem
	prefixes := []string{"TODO:", "TODO ", "ACTION:", "ACTION ", "TASK:", "TASK ", "- [ ] "}

	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		upper := strings.ToUpper(t)

		hit := strings.HasPrefix(upper, "TODO") || strings.HasPrefix(upper, "ACTION") ||
			strings.HasPrefix(upper, "TASK") || strings.Contains(t, "[ ]")
		if !hit {
			continue
		}

		clean := t
		for _, p := range prefixes {
			if strings.HasPrefix(upper, strings.ToUpper(p)) {
				clean = strings.TrimSpace(t[len(p):])
				break
			}
		}
		items = append(items, actionItem{Text: clean, Priority: "medium"})
	}
	return items
}
