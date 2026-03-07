// Package doc_perceiver provides document perception skill.
package doc_perceiver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"harmonclaw/skills"
)

func init() {
	skills.Register(&Perceiver{})
}

var (
	reWord      = regexp.MustCompile(`[\p{L}\p{N}_]{2,}`)
	maxSummary  = 500
)

type Perceiver struct{}

func (p *Perceiver) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: "doc_perceiver", Version: "0.2.0", Core: "architect"}
}

func (p *Perceiver) Execute(input skills.SkillInput) skills.SkillOutput {
	start := time.Now()

	content := input.Text
	fileType := "text"

	if dir := getDir(input); dir != "" {
		results, err := scanDir(dir)
		if err != nil {
			return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "dir scan: " + err.Error()}
		}
		data, _ := json.Marshal(map[string]any{"files": results})
		out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: data}
		out.Metrics.Ms = time.Since(start).Milliseconds()
		out.Metrics.Bytes = len(data)
		return out
	}
	if path := getPath(input); path != "" {
		if isDir(path) {
			results, err := scanDir(path)
			if err != nil {
				return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "dir scan: " + err.Error()}
			}
			data, _ := json.Marshal(map[string]any{"files": results})
			out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: data}
			out.Metrics.Ms = time.Since(start).Milliseconds()
			out.Metrics.Bytes = len(data)
			return out
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "read file: " + err.Error()}
		}
		content = string(data)
		fileType = detectFileType(path)
	}

	if content == "" {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "no content"}
	}

	result := docResult{
		Title:     extractTitle(content),
		Summary:   extractSummary(content),
		Keywords:  extractKeywords(content),
		WordCount: wordCount(content),
		FileType:  fileType,
	}

	data, _ := json.Marshal(result)
	out := skills.SkillOutput{TraceID: input.TraceID, Status: "ok", Data: data}
	out.Metrics.Ms = time.Since(start).Milliseconds()
	out.Metrics.Bytes = len(data)
	out.Metrics.Tokens = len(content) / 4
	return out
}

func getPath(input skills.SkillInput) string {
	if input.Args == nil {
		return ""
	}
	if p := input.Args["path"]; p != "" {
		return safePath(p)
	}
	if p := input.Args["file"]; p != "" {
		return safePath(p)
	}
	return ""
}

func getDir(input skills.SkillInput) string {
	if input.Args == nil {
		return ""
	}
	if d := input.Args["dir"]; d == "" {
		return ""
	}
	return safePath(input.Args["dir"])
}

func safePath(p string) string {
	if p == "" {
		return ""
	}
	if strings.Contains(p, "..") {
		return ""
	}
	return filepath.Clean(p)
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func detectFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt":
		return "text"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	default:
		return ext
	}
}

func scanDir(dir string) ([]docResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var results []docResult
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if !strings.HasSuffix(name, ".txt") && !strings.HasSuffix(name, ".md") && !strings.HasSuffix(name, ".json") {
			continue
		}
		fpath := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(fpath)
		if err != nil {
			continue
		}
		content := string(data)
		results = append(results, docResult{
			Title:     extractTitle(content),
			Summary:   extractSummary(content),
			Keywords:  extractKeywords(content),
			WordCount: wordCount(content),
			FileType:  detectFileType(fpath),
		})
	}
	return results, nil
}

type docResult struct {
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	Keywords  []string `json:"keywords"`
	WordCount int      `json:"word_count"`
	FileType  string   `json:"file_type"`
}

func extractTitle(text string) string {
	for _, line := range strings.SplitN(text, "\n", 10) {
		t := strings.TrimSpace(line)
		if t != "" {
			return strings.TrimSpace(strings.TrimLeft(t, "#"))
		}
	}
	return "Untitled"
}

func extractSummary(text string) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= maxSummary {
		return text
	}
	return string(runes[:maxSummary]) + "..."
}

func extractKeywords(text string) []string {
	seen := make(map[string]bool)
	var kw []string
	for _, m := range reWord.FindAllString(text, 50) {
		m = strings.ToLower(m)
		if len(m) >= 2 && !seen[m] {
			seen[m] = true
			kw = append(kw, m)
		}
		if len(kw) >= 15 {
			break
		}
	}
	return kw
}

func wordCount(text string) int {
	n := 0
	for _, w := range strings.Fields(text) {
		if len(w) > 0 {
			n++
		}
	}
	return n
}
