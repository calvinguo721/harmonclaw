// Package doc_perceiver provides document perception skill.
package doc_perceiver

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"io"
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
	reWord       = regexp.MustCompile(`[\p{L}\p{N}_]{2,}`)
	maxSummary   = 500
	maxFileBytes = 1024 * 1024
	stopWords    = map[string]bool{"the": true, "a": true, "an": true, "is": true, "are": true, "的": true, "了": true, "是": true, "在": true}
	sentenceEnd  = regexp.MustCompile(`[。！？.!?]`)
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
		data, err := readFileLimit(path, maxFileBytes)
		if err != nil {
			return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "read file: " + err.Error()}
		}
		content, fileType = parseContent(path, data)
	}

	if content == "" {
		return skills.SkillOutput{TraceID: input.TraceID, Status: "error", Error: "no content"}
	}

	result := docResult{
		Title:     extractTitle(content),
		Summary:   extractSummarySentences(content),
		Keywords:  extractKeywordsTop(content, 10),
		Entities:  extractEntities(content),
		WordCount: wordCount(content),
		FileType:  fileType,
		FileHash:  hashContent(content),
		FileSize:  len(content),
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
	case ".html", ".htm":
		return "html"
	case ".csv":
		return "csv"
	default:
		return ext
	}
}

func readFileLimit(path string, limit int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, int64(limit)))
}

func parseContent(path string, data []byte) (string, string) {
	ft := detectFileType(path)
	switch ft {
	case "html":
		return parseHTML(data), ft
	case "csv":
		return parseCSV(data), ft
	default:
		return string(data), ft
	}
}

func parseHTML(data []byte) string {
	content := string(data)
	start := strings.Index(content, "<body")
	if start < 0 {
		start = 0
	} else {
		start = strings.Index(content[start:], ">") + start + 1
	}
	end := strings.Index(content, "</body>")
	if end < 0 {
		end = len(content)
	}
	body := content[start:end]
	body = regexp.MustCompile(`(?i)<script[^>]*>[\s\S]*?</script>`).ReplaceAllString(body, "")
	body = regexp.MustCompile(`(?i)<style[^>]*>[\s\S]*?</style>`).ReplaceAllString(body, "")
	body = regexp.MustCompile(`(?i)<nav[^>]*>[\s\S]*?</nav>`).ReplaceAllString(body, "")
	body = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(body, " ")
	body = regexp.MustCompile(`\s+`).ReplaceAllString(body, " ")
	return strings.TrimSpace(body)
}

func parseCSV(data []byte) string {
	r := csv.NewReader(bytes.NewReader(data))
	var b strings.Builder
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return string(data)
		}
		b.WriteString(strings.Join(row, " | "))
		b.WriteString("\n")
	}
	return b.String()
}

func hashContent(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func extractSummarySentences(text string) string {
	parts := sentenceEnd.Split(text, 4)
	var b strings.Builder
	for i := 0; i < 3 && i < len(parts); i++ {
		p := strings.TrimSpace(parts[i])
		if p != "" {
			if b.Len() > 0 {
				b.WriteString("。")
			}
			b.WriteString(p)
		}
	}
	return b.String()
}

func extractKeywordsTop(text string, n int) []string {
	freq := make(map[string]int)
	for _, m := range reWord.FindAllString(text, 100) {
		m = strings.ToLower(m)
		if len(m) >= 2 && !stopWords[m] {
			freq[m]++
		}
	}
	var kw []string
	for k := range freq {
		kw = append(kw, k)
	}
	for i := 0; i < len(kw)-1; i++ {
		for j := i + 1; j < len(kw); j++ {
			if freq[kw[j]] > freq[kw[i]] {
				kw[i], kw[j] = kw[j], kw[i]
			}
		}
	}
	if len(kw) > n {
		kw = kw[:n]
	}
	return kw
}

var (
	reDate  = regexp.MustCompile(`\d{4}[-/]\d{1,2}[-/]\d{1,2}`)
	reNum   = regexp.MustCompile(`\d+(\.\d+)?`)
	reName  = regexp.MustCompile(`[A-Z][a-z]+\s+[A-Z][a-z]+`)
)

func extractEntities(text string) []string {
	var out []string
	for _, m := range reDate.FindAllString(text, -1) {
		out = append(out, m)
	}
	for _, m := range reNum.FindAllString(text, -1) {
		if len(m) <= 15 {
			out = append(out, m)
		}
	}
	for _, m := range reName.FindAllString(text, -1) {
		out = append(out, m)
	}
	return out
}

func scanDir(dir string) ([]docResult, error) {
	return scanDirRec(dir, nil)
}

func scanDirRec(dir string, results []docResult) ([]docResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return results, err
	}
	for _, e := range entries {
		fpath := filepath.Join(dir, e.Name())
		if e.IsDir() {
			results, _ = scanDirRec(fpath, results)
			continue
		}
		name := strings.ToLower(e.Name())
		if !strings.HasSuffix(name, ".txt") && !strings.HasSuffix(name, ".md") && !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".html") && !strings.HasSuffix(name, ".htm") && !strings.HasSuffix(name, ".csv") {
			continue
		}
		data, err := readFileLimit(fpath, maxFileBytes)
		if err != nil {
			continue
		}
		content, _ := parseContent(fpath, data)
		results = append(results, docResult{
			Title:     extractTitle(content),
			Summary:   extractSummarySentences(content),
			Keywords:  extractKeywordsTop(content, 10),
			Entities:  extractEntities(content),
			WordCount: wordCount(content),
			FileType:  detectFileType(fpath),
			FileHash:  hashContent(content),
			FileSize:  len(content),
		})
	}
	return results, nil
}

type docResult struct {
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	Keywords  []string `json:"keywords"`
	Entities  []string `json:"entities"`
	WordCount int      `json:"word_count"`
	FileType  string   `json:"file_type"`
	FileHash  string   `json:"file_hash"`
	FileSize  int      `json:"file_size"`
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
