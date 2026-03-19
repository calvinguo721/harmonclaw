// Package engine provides short-term and long-term memory with extraction and retrieval.
package engine

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Engram is a memory unit for long-term storage.
type Engram struct {
	ID           string
	SessionID    string
	Timestamp    time.Time
	Content      string
	Summary      string
	Keywords     []string
	Entities     []string
	Importance   int
	LastAccessed time.Time
	AccessCount  int
}

// LongTermStore persists and retrieves engrams.
type LongTermStore interface {
	Write(e Engram) error
	Search(query string, from, to time.Time, limit int) []Engram
	UpdateAccess(id string, t time.Time)
}

// MemoryEngine manages short-term (session) and long-term memory.
type MemoryEngine struct {
	mu        sync.RWMutex
	shortTerm sync.Map // sessionID -> []Turn
	store     LongTermStore
	decayDays int
}

// NewMemoryEngine creates a memory engine.
func NewMemoryEngine(store LongTermStore) *MemoryEngine {
	return &MemoryEngine{
		store:     store,
		decayDays: 30,
	}
}

// AppendShortTerm adds a turn to short-term memory.
func (m *MemoryEngine) AppendShortTerm(sessionID, role, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, _ := m.shortTerm.LoadOrStore(sessionID, &[]Turn{})
	sl := v.(*[]Turn)
	n := make([]Turn, len(*sl)+1)
	copy(n, *sl)
	n[len(n)-1] = Turn{Role: role, Content: content, Timestamp: time.Now()}
	m.shortTerm.Store(sessionID, &n)
}

func (m *MemoryEngine) getShortTerm(sessionID string) []Turn {
	v, ok := m.shortTerm.Load(sessionID)
	if !ok {
		return nil
	}
	sl := v.(*[]Turn)
	out := make([]Turn, len(*sl))
	copy(out, *sl)
	return out
}

// FlushSession writes short-term to long-term and clears short-term.
func (m *MemoryEngine) FlushSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	turns := m.getShortTerm(sessionID)
	if len(turns) == 0 || m.store == nil {
		m.shortTerm.Delete(sessionID)
		return
	}

	summary := extractSummary(turns)
	keywords := extractKeywords(summary)
	entities := extractEntities(summary)
	importance := computeImportance(keywords, entities)

	e := Engram{
		ID:           genID(),
		SessionID:    sessionID,
		Timestamp:    time.Now(),
		Content:      formatTurns(turns),
		Summary:      summary,
		Keywords:     keywords,
		Entities:     entities,
		Importance:   importance,
		LastAccessed: time.Now(),
		AccessCount:  0,
	}
	_ = m.store.Write(e)
	m.shortTerm.Delete(sessionID)
}

// WriteEngram implements MemoryWriter. Writes summary to long-term store.
func (m *MemoryEngine) WriteEngram(sessionID, summary string) error {
	if m.store == nil {
		return nil
	}
	keywords := extractKeywords(summary)
	entities := extractEntities(summary)
	e := Engram{
		ID:           genID(),
		SessionID:    sessionID,
		Timestamp:    time.Now(),
		Content:      summary,
		Summary:      summary,
		Keywords:     keywords,
		Entities:     entities,
		Importance:   computeImportance(keywords, entities),
		LastAccessed: time.Now(),
	}
	return m.store.Write(e)
}

// Search retrieves relevant memories with decay and merge.
func (m *MemoryEngine) Search(query string, limit int) []string {
	if m.store == nil {
		return nil
	}
	now := time.Now()
	from := now.AddDate(0, 0, -90)
	engrams := m.store.Search(query, from, now, limit*2)
	if len(engrams) == 0 {
		return nil
	}

	scored := make([]struct {
		e     Engram
		score float64
	}, 0, len(engrams))
	for _, e := range engrams {
		score := 1.0
		if e.LastAccessed.Before(now.AddDate(0, 0, -m.decayDays)) {
			score *= 0.5
		}
		score *= float64(e.Importance) / 5.0
		if score < 0.2 {
			score = 0.2
		}
		scored = append(scored, struct {
			e     Engram
			score float64
		}{e, score})
	}
	// sort by score desc
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	merged := mergeSimilar(scored, 0.4)
	out := make([]string, 0, limit)
	for i, x := range merged {
		if i >= limit {
			break
		}
		m.store.UpdateAccess(x.e.ID, now)
		out = append(out, x.e.Summary)
	}
	return out
}

func extractSummary(turns []Turn) string {
	var b strings.Builder
	for _, t := range turns {
		b.WriteString(t.Content)
		b.WriteString(" ")
	}
	return strings.TrimSpace(b.String())
}

func extractKeywords(text string) []string {
	stopWords := map[string]bool{
		"的": true, "了": true, "是": true, "在": true, "我": true, "有": true,
		"和": true, "就": true, "不": true, "人": true, "都": true, "一": true,
		"the": true, "a": true, "an": true, "is": true, "are": true,
	}
	freq := make(map[string]int)
	tokens := tokenize(text)
	for _, t := range tokens {
		t = strings.ToLower(t)
		if len(t) < 2 || stopWords[t] {
			continue
		}
		freq[t]++
	}
	var out []string
	for k, c := range freq {
		if c >= 1 {
			out = append(out, k)
		}
	}
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

var (
	dateRe  = regexp.MustCompile(`\d{4}[-/]\d{1,2}[-/]\d{1,2}|\d{1,2}月\d{1,2}日`)
	numRe   = regexp.MustCompile(`\d+(\.\d+)?`)
	nameRe  = regexp.MustCompile(`[A-Z][a-z]+\s+[A-Z][a-z]+`)
)

func extractEntities(text string) []string {
	var out []string
	for _, m := range dateRe.FindAllString(text, -1) {
		out = append(out, m)
	}
	for _, m := range numRe.FindAllString(text, -1) {
		if len(m) <= 10 {
			out = append(out, m)
		}
	}
	for _, m := range nameRe.FindAllString(text, -1) {
		out = append(out, m)
	}
	return out
}

func computeImportance(keywords, entities []string) int {
	n := len(keywords) + len(entities)*2
	if n >= 10 {
		return 5
	}
	if n >= 6 {
		return 4
	}
	if n >= 3 {
		return 3
	}
	if n >= 1 {
		return 2
	}
	return 1
}

func formatTurns(turns []Turn) string {
	var b strings.Builder
	for _, t := range turns {
		b.WriteString(t.Role)
		b.WriteString(": ")
		b.WriteString(t.Content)
		b.WriteString("\n")
	}
	return b.String()
}

var idCounter int64

func genID() string {
	n := atomic.AddInt64(&idCounter, 1)
	return time.Now().Format("20060102150405") + "-" + fmt.Sprintf("%06x", n%0xffffff)
}

func jaccard(a, b []string) float64 {
	setA := make(map[string]bool)
	for _, x := range a {
		setA[x] = true
	}
	setB := make(map[string]bool)
	for _, x := range b {
		setB[x] = true
	}
	inter := 0
	for k := range setA {
		if setB[k] {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func mergeSimilar(scored []struct {
	e     Engram
	score float64
}, threshold float64) []struct {
	e     Engram
	score float64
} {
	var out []struct {
		e     Engram
		score float64
	}
	for _, s := range scored {
		merged := false
		kw := append([]string{}, s.e.Keywords...)
		kw = append(kw, s.e.Entities...)
		for i := range out {
			ow := append([]string{}, out[i].e.Keywords...)
			ow = append(ow, out[i].e.Entities...)
			if jaccard(kw, ow) >= threshold {
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, s)
		}
	}
	return out
}
