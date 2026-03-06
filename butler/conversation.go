// Package butler (conversation) provides multi-turn conversation with context window.
package butler

import (
	"sync"

	"harmonclaw/llm"
	"harmonclaw/viking"
)

const defaultContextWindow = 20

// ConversationManager manages multi-turn dialogue with sliding context.
type ConversationManager struct {
	mu       sync.Mutex
	memory   viking.MemoryWithHistory
	window   int
	sessions map[string][]llm.Message
}

// NewConversationManager creates a manager.
func NewConversationManager(mem viking.MemoryWithHistory) *ConversationManager {
	return &ConversationManager{
		memory:   mem,
		window:   defaultContextWindow,
		sessions: make(map[string][]llm.Message),
	}
}

// SetContextWindow sets max messages to keep in context.
func (c *ConversationManager) SetContextWindow(n int) {
	if n < 2 {
		n = 2
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.window = n
}

// Append adds a message and returns the context for LLM.
func (c *ConversationManager) Append(user, sessionID, role, content string) ([]llm.Message, error) {
	if c.memory != nil {
		if err := c.memory.SaveMemory(user, sessionID, role, content); err != nil {
			return nil, err
		}
	}
	c.mu.Lock()
	key := user + ":" + sessionID
	msgs := c.sessions[key]
	msgs = append(msgs, llm.Message{Role: role, Content: content})
	if len(msgs) > c.window {
		msgs = msgs[len(msgs)-c.window:]
	}
	c.sessions[key] = msgs
	ctx := make([]llm.Message, len(msgs))
	copy(ctx, msgs)
	c.mu.Unlock()
	return ctx, nil
}

// LoadContext loads history from Viking and returns messages for LLM.
func (c *ConversationManager) LoadContext(user, sessionID string) ([]llm.Message, error) {
	if c.memory == nil {
		return nil, nil
	}
	entries, err := c.memory.LoadHistory(user, sessionID)
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	c.mu.Lock()
	key := user + ":" + sessionID
	msgs := make([]llm.Message, 0, len(entries))
	for _, e := range entries {
		msgs = append(msgs, llm.Message{Role: e.Role, Content: e.Content})
	}
	if len(msgs) > c.window {
		msgs = msgs[len(msgs)-c.window:]
	}
	c.sessions[key] = msgs
	ctx := make([]llm.Message, len(msgs))
	copy(ctx, msgs)
	c.mu.Unlock()
	return ctx, nil
}

// GetContext returns current context for session without loading.
func (c *ConversationManager) GetContext(user, sessionID string) []llm.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.sessions[user+":"+sessionID]
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	return out
}
