// Package viking provides Chinese segmentation and English stemming.
package viking

import (
	"bufio"
	"os"
	"strings"
	"sync"
	"unicode"
)

// Segmenter does Chinese forward max match + English tokenization.
type Segmenter struct {
	mu   sync.RWMutex
	dict map[string]bool
	max  int
}

// NewSegmenter creates a segmenter. Loads dict from path if exists.
func NewSegmenter(dictPath string) *Segmenter {
	s := &Segmenter{dict: make(map[string]bool), max: 4}
	if dictPath != "" {
		s.loadDict(dictPath)
	}
	if len(s.dict) == 0 {
		s.dict = defaultDict()
	}
	for w := range s.dict {
		if len([]rune(w)) > s.max {
			s.max = len([]rune(w))
		}
	}
	return s
}

func (s *Segmenter) loadDict(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		w := strings.TrimSpace(sc.Text())
		if w != "" && w[0] != '#' {
			s.dict[w] = true
			if n := len([]rune(w)); n > s.max {
				s.max = n
			}
		}
	}
}

func defaultDict() map[string]bool {
	words := []string{"的", "了", "是", "在", "我", "有", "和", "就", "不", "人", "都", "一",
		"一个", "上", "也", "很", "到", "说", "要", "去", "你", "会", "能", "与", "及",
		"搜索", "查找", "文档", "语言", "性能", "优化", "项目", "代码", "测试", "开发",
		"the", "a", "an", "is", "are", "was", "were", "be", "been", "being",
		"have", "has", "had", "do", "does", "did", "will", "would", "could",
		"search", "find", "document", "language", "code", "test", "project"}
	m := make(map[string]bool)
	for _, w := range words {
		m[w] = true
	}
	return m
}

// Segment tokenizes text: Chinese via forward max match, English via space + stem.
func (s *Segmenter) Segment(text string) []string {
	s.mu.RLock()
	dict := s.dict
	maxLen := s.max
	s.mu.RUnlock()

	var out []string
	runes := []rune(text)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if r >= 0x4e00 && r <= 0x9fff {
			matched := false
			for w := min(maxLen, len(runes)-i); w >= 1; w-- {
				if i+w > len(runes) {
					continue
				}
				seg := string(runes[i : i+w])
				if dict[seg] {
					out = append(out, seg)
					i += w
					matched = true
					break
				}
			}
			if !matched {
				out = append(out, string(r))
				i++
			}
		} else if (unicode.IsLetter(r) || unicode.IsNumber(r)) && !(r >= 0x4e00 && r <= 0x9fff) {
			j := i
			for j < len(runes) {
				rr := runes[j]
				if (unicode.IsLetter(rr) || unicode.IsNumber(rr)) && !(rr >= 0x4e00 && rr <= 0x9fff) {
					j++
				} else {
					break
				}
			}
			word := string(runes[i:j])
			stemmed := porterStem(strings.ToLower(word))
			if stemmed != "" {
				out = append(out, stemmed)
			}
			i = j
		} else {
			i++
		}
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// porterStem is a simplified Porter stemmer (stdlib only).
func porterStem(word string) string {
	if len(word) < 3 {
		return word
	}
	// Step 1a: sses->ss, ies->i, ss->ss, s->"
	if strings.HasSuffix(word, "sses") {
		return word[:len(word)-2]
	}
	if strings.HasSuffix(word, "ies") {
		return word[:len(word)-3] + "i"
	}
	if strings.HasSuffix(word, "ss") {
		return word
	}
	if strings.HasSuffix(word, "s") && len(word) > 3 {
		return word[:len(word)-1]
	}
	// Step 1b: eed->ee if m>0
	if strings.HasSuffix(word, "eed") {
		return word[:len(word)-1]
	}
	// ed, ing
	if strings.HasSuffix(word, "ed") && len(word) > 4 {
		return word[:len(word)-2]
	}
	if strings.HasSuffix(word, "ing") && len(word) > 5 {
		return word[:len(word)-3]
	}
	// Step 2: ational->ate, tional->tion
	if strings.HasSuffix(word, "ational") {
		return word[:len(word)-5] + "e"
	}
	if strings.HasSuffix(word, "tion") {
		return word[:len(word)-1]
	}
	return word
}
