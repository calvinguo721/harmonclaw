// Package viking (search) provides full-text search with persist to index.jsonl.
package viking

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

// SearchIndex is an inverted index with persist to index.jsonl.
type SearchIndex struct {
	mu      sync.RWMutex
	index   map[string]map[string]int
	docs    map[string]string
	docMeta map[string]map[string]string
	path    string
}

// NewSearchIndex creates an index.
func NewSearchIndex() *SearchIndex {
	return &SearchIndex{
		index:   make(map[string]map[string]int),
		docs:    make(map[string]string),
		docMeta: make(map[string]map[string]string),
	}
}

// NewSearchIndexWithPath creates an index that persists to path.
func NewSearchIndexWithPath(indexPath string) *SearchIndex {
	s := NewSearchIndex()
	s.path = indexPath
	s.Load()
	return s
}

type indexLine struct {
	DocID   string            `json:"doc_id"`
	Content string            `json:"content"`
	Meta    map[string]string `json:"meta"`
}

// Load reads index from index.jsonl.
func (s *SearchIndex) Load() error {
	if s.path == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		var line indexLine
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		s.docs[line.DocID] = line.Content
		s.docMeta[line.DocID] = line.Meta
		for _, t := range tokenize(strings.ToLower(line.Content)) {
			if t == "" {
				continue
			}
			if s.index[t] == nil {
				s.index[t] = make(map[string]int)
			}
			s.index[t][line.DocID]++
		}
	}
	return nil
}

// Persist writes index to index.jsonl.
func (s *SearchIndex) Persist() error {
	if s.path == "" {
		return nil
	}
	s.mu.RLock()
	lines := make([]indexLine, 0, len(s.docs))
	for id, content := range s.docs {
		lines = append(lines, indexLine{
			DocID:   id,
			Content: content,
			Meta:    s.docMeta[id],
		})
	}
	s.mu.RUnlock()
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, l := range lines {
		if err := enc.Encode(l); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, s.path)
}

func tokenize(text string) []string {
	var tokens []string
	var buf []rune
	for _, r := range text {
		isCJK := r >= 0x4e00 && r <= 0x9fff
		if unicode.IsLetter(r) || unicode.IsNumber(r) || isCJK {
			if isCJK {
				if len(buf) > 0 {
					tokens = append(tokens, string(buf))
					buf = nil
				}
				tokens = append(tokens, string(r))
			} else {
				buf = append(buf, r)
			}
		} else {
			if len(buf) > 0 {
				tokens = append(tokens, string(buf))
				buf = nil
			}
		}
	}
	if len(buf) > 0 {
		tokens = append(tokens, string(buf))
	}
	return tokens
}

// Index adds or updates a document.
func (s *SearchIndex) Index(docID, content string, meta map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.docs[docID]; ok {
		for _, t := range tokenize(strings.ToLower(old)) {
			if m := s.index[t]; m != nil {
				delete(m, docID)
				if len(m) == 0 {
					delete(s.index, t)
				}
			}
		}
	}
	s.docs[docID] = content
	s.docMeta[docID] = meta
	for _, t := range tokenize(strings.ToLower(content)) {
		if t == "" {
			continue
		}
		if s.index[t] == nil {
			s.index[t] = make(map[string]int)
		}
		s.index[t][docID]++
	}
}

// Search returns docIDs matching query.
func (s *SearchIndex) Search(query string) []string {
	tokens := tokenize(strings.ToLower(query))
	if len(tokens) == 0 {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	score := make(map[string]int)
	for _, t := range tokens {
		for docID, cnt := range s.index[t] {
			score[docID] += cnt
		}
	}
	var result []string
	for docID := range score {
		result = append(result, docID)
	}
	return result
}

// Get returns document content.
func (s *SearchIndex) Get(docID string) (string, map[string]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	content, ok := s.docs[docID]
	if !ok {
		return "", nil, false
	}
	meta := s.docMeta[docID]
	return content, meta, true
}
