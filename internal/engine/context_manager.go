// Package engine provides session context management.
package engine

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Turn is a single conversation turn.
type Turn struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// MemoryWriter writes summary to long-term storage.
type MemoryWriter interface {
	WriteEngram(sessionID, summary string) error
}

// MemoryRetriever retrieves relevant memories.
type MemoryRetriever interface {
	Search(query string, limit int) []string
}

const (
	defaultWindowSize   = 20
	summaryThreshold    = 10
	injectRounds        = 5
	sessionTimeoutMins = 30
)

// ContextManager maintains per-session conversation history.
type ContextManager struct {
	mu             sync.RWMutex
	sessions       map[string]*sessionState
	windowSize     int
	writer         MemoryWriter
	retriever      MemoryRetriever
	lastCleanup    time.Time
	SessionTimeout time.Duration
}

type sessionState struct {
	turns      []Turn
	lastAccess time.Time
}

// NewContextManager creates a context manager.
func NewContextManager(writer MemoryWriter, retriever MemoryRetriever) *ContextManager {
	return &ContextManager{
		sessions:   make(map[string]*sessionState),
		windowSize: defaultWindowSize,
		writer:     writer,
		retriever:  retriever,
	}
}

// Append adds a turn and returns recent history. Triggers summary if over threshold.
func (c *ContextManager) Append(sessionID, role, content string) ([]Turn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	s := c.sessions[sessionID]
	if s == nil {
		s = &sessionState{turns: nil, lastAccess: time.Now()}
		c.sessions[sessionID] = s
	}
	s.lastAccess = time.Now()

	s.turns = append(s.turns, Turn{Role: role, Content: content, Timestamp: time.Now()})

	if len(s.turns) > c.windowSize {
		excess := len(s.turns) - c.windowSize
		toRemove := excess
		if toRemove > summaryThreshold {
			toRemove = summaryThreshold
		}
		toSummarize := s.turns[:toRemove]
		s.turns = s.turns[toRemove:]
		if c.writer != nil {
			summary := c.summarizeTurns(toSummarize)
			_ = c.writer.WriteEngram(sessionID, summary)
		}
	}

	out := make([]Turn, len(s.turns))
	copy(out, s.turns)
	return out, nil
}

func (c *ContextManager) summarizeTurns(turns []Turn) string {
	var b strings.Builder
	for _, t := range turns {
		b.WriteString(t.Role)
		b.WriteString(": ")
		b.WriteString(t.Content)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// GetRecent returns the last n turns for a session.
func (c *ContextManager) GetRecent(sessionID string, n int) []Turn {
	c.mu.RLock()
	defer c.mu.RUnlock()

	s := c.sessions[sessionID]
	if s == nil {
		return nil
	}
	s.lastAccess = time.Now()

	if n <= 0 || n > len(s.turns) {
		n = len(s.turns)
	}
	start := len(s.turns) - n
	if start < 0 {
		start = 0
	}
	out := make([]Turn, n)
	copy(out, s.turns[start:])
	return out
}

// GetContextForLLM returns recent turns + retrieved memories for LLM injection.
func (c *ContextManager) GetContextForLLM(sessionID, query string) (turns []Turn, memories []string) {
	c.mu.RLock()
	s := c.sessions[sessionID]
	if s != nil {
		s.lastAccess = time.Now()
	}
	c.mu.RUnlock()

	turns = c.GetRecent(sessionID, injectRounds)

	if c.retriever != nil && query != "" {
		memories = c.retriever.Search(query, 5)
	}
	return turns, memories
}

// ArchiveStale removes sessions idle for more than SessionTimeout.
func (c *ContextManager) ArchiveStale() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	timeout := c.SessionTimeout
	if timeout == 0 {
		timeout = sessionTimeoutMins * time.Minute
	}
	cutoff := time.Now().Add(-timeout)
	n := 0
	for id, s := range c.sessions {
		if s.lastAccess.Before(cutoff) {
			delete(c.sessions, id)
			n++
		}
	}
	c.lastCleanup = time.Now()
	return n
}

// SessionCount returns the number of active sessions.
func (c *ContextManager) SessionCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.sessions)
}

// FormatContextForPrompt formats turns and memories for LLM prompt.
func FormatContextForPrompt(turns []Turn, memories []string) string {
	var b strings.Builder
	if len(memories) > 0 {
		b.WriteString("Relevant memories:\n")
		for _, m := range memories {
			b.WriteString("- ")
			b.WriteString(m)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Recent conversation:\n")
	for _, t := range turns {
		b.WriteString(fmt.Sprintf("%s: %s\n", t.Role, t.Content))
	}
	return b.String()
}
