// Package engine provides in-memory long-term store for testing.
package engine

import (
	"strings"
	"sync"
	"time"
)

// MemStore is an in-memory LongTermStore.
type MemStore struct {
	mu      sync.RWMutex
	engrams map[string]Engram
}

// NewMemStore creates an in-memory store.
func NewMemStore() *MemStore {
	return &MemStore{engrams: make(map[string]Engram)}
}

// Write stores an engram.
func (s *MemStore) Write(e Engram) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.ID == "" {
		e.ID = genID()
	}
	s.engrams[e.ID] = e
	return nil
}

// Search returns engrams matching query.
func (s *MemStore) Search(query string, from, to time.Time, limit int) []Engram {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(query)
	var out []Engram
	for _, e := range s.engrams {
		if e.Timestamp.Before(from) || e.Timestamp.After(to) {
			continue
		}
		content := strings.ToLower(e.Content + " " + e.Summary + " " + strings.Join(e.Keywords, " "))
		if query != "" && !strings.Contains(content, q) {
			continue
		}
		out = append(out, e)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// UpdateAccess updates last accessed time.
func (s *MemStore) UpdateAccess(id string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.engrams[id]; ok {
		e.LastAccessed = t
		e.AccessCount++
		s.engrams[id] = e
	}
}
