// Package viking (search) provides local full-text search with inverted index.
package viking

import (
	"strings"
	"sync"
	"unicode"
)

// SearchIndex is an in-memory inverted index.
type SearchIndex struct {
	mu      sync.RWMutex
	index   map[string]map[string]int // term -> docID -> count
	docs    map[string]string         // docID -> content
	docMeta map[string]map[string]string
}

// NewSearchIndex creates an index.
func NewSearchIndex() *SearchIndex {
	return &SearchIndex{
		index:   make(map[string]map[string]int),
		docs:    make(map[string]string),
		docMeta: make(map[string]map[string]string),
	}
}

// tokenize splits text into tokens. Uses unicode for Chinese/letter boundaries.
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
