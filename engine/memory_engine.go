// Package engine provides memory engine with short/long-term and decay (OpenClaw-style).
package engine

import (
	"strings"
	"time"
)

// MemoryEntry is a retrievable memory unit.
type MemoryEntry struct {
	ID            string
	Content       string
	Classification string
	CreatedAt     time.Time
	UserID        string
	SessionID     string
}

// MemoryRetriever can search memories.
type MemoryRetriever interface {
	Search(query string, limit int) []MemoryEntry
}

// MemoryEngine combines short-term buffer and long-term retrieval with decay.
type MemoryEngine struct {
	shortTerm []MemoryEntry
	maxShort  int
	retriever MemoryRetriever
}

// NewMemoryEngine creates an engine. retriever may be nil.
func NewMemoryEngine(maxShort int, retriever MemoryRetriever) *MemoryEngine {
	if maxShort <= 0 {
		maxShort = 20
	}
	return &MemoryEngine{
		shortTerm: nil,
		maxShort:  maxShort,
		retriever: retriever,
	}
}

// PushShort adds to short-term buffer (recent conversation).
func (e *MemoryEngine) PushShort(entry MemoryEntry) {
	entry.CreatedAt = time.Now()
	e.shortTerm = append(e.shortTerm, entry)
	if len(e.shortTerm) > e.maxShort {
		e.shortTerm = e.shortTerm[len(e.shortTerm)-e.maxShort:]
	}
}

// ShortTerm returns recent entries.
func (e *MemoryEngine) ShortTerm() []MemoryEntry {
	out := make([]MemoryEntry, len(e.shortTerm))
	copy(out, e.shortTerm)
	return out
}

// DecayFactor returns relevance decay for age (0..1). Exponential: 1 at t=0, 0.5 at halfLife.
func DecayFactor(age time.Duration, halfLife time.Duration) float64 {
	if halfLife <= 0 {
		halfLife = 24 * time.Hour
	}
	if age <= 0 {
		return 1.0
	}
	// 2^(-age/halfLife)
	ratio := float64(age) / float64(halfLife)
	decay := 1.0
	for ratio >= 1 {
		decay *= 0.5
		ratio--
	}
	if ratio > 0 {
		decay *= 1.0 - ratio*0.5
	}
	if decay < 1e-6 {
		return 1e-6
	}
	return decay
}

// Retrieve returns memories for query, short-term first then long-term with decay.
func (e *MemoryEngine) Retrieve(query string, limit int) []MemoryEntry {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}
	var out []MemoryEntry
	now := time.Now()
	halfLife := 7 * 24 * time.Hour

	for i := len(e.shortTerm) - 1; i >= 0 && len(out) < limit; i-- {
		ent := e.shortTerm[i]
		if strings.Contains(strings.ToLower(ent.Content), query) {
			out = append(out, ent)
		}
	}

	if e.retriever != nil && len(out) < limit {
		long := e.retriever.Search(query, limit-len(out))
		for _, ent := range long {
			age := now.Sub(ent.CreatedAt)
			if DecayFactor(age, halfLife) > 0.01 {
				out = append(out, ent)
			}
		}
	}
	return out
}
