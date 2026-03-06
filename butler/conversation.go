// Package butler (conversation) provides per-user history with sliding window and persistence.
package butler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"harmonclaw/llm"
	"harmonclaw/viking"
)

const (
	defaultContextWindow = 20
	conversationsDir     = "conversations"
)

// ConversationManager manages per-user dialogue with sliding window and JSONL persistence.
type ConversationManager struct {
	mu       sync.Mutex
	memory   viking.MemoryWithHistory
	baseDir  string
	window   int
	sessions map[string][]llm.Message
}

// NewConversationManager creates a manager. baseDir is ~/.harmonclaw/viking.
func NewConversationManager(mem viking.MemoryWithHistory, baseDir string) *ConversationManager {
	cm := &ConversationManager{
		memory:   mem,
		baseDir:  baseDir,
		window:   defaultContextWindow,
		sessions: make(map[string][]llm.Message),
	}
	cm.restoreAll()
	return cm
}

func (c *ConversationManager) convPath(userID string) string {
	return filepath.Join(c.baseDir, conversationsDir, userID+".jsonl")
}

func (c *ConversationManager) restoreAll() {
	dir := filepath.Join(c.baseDir, conversationsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) > 5 && name[len(name)-5:] == ".jsonl" {
			userID := name[:len(name)-5]
			msgs := c.loadFromFile(c.convPath(userID))
			if len(msgs) > 0 {
				c.mu.Lock()
				if len(msgs) > c.window {
					msgs = msgs[len(msgs)-c.window:]
				}
				c.sessions[userID] = msgs
				c.mu.Unlock()
			}
		}
	}
}

func (c *ConversationManager) loadFromFile(path string) []llm.Message {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var msgs []llm.Message
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m llm.Message
		if json.Unmarshal(sc.Bytes(), &m) == nil && m.Role != "" {
			msgs = append(msgs, m)
		}
	}
	return msgs
}

func (c *ConversationManager) persist(userID string, msgs []llm.Message) error {
	dir := filepath.Join(c.baseDir, conversationsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := c.convPath(userID)
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, m := range msgs {
		if err := enc.Encode(m); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	if err := f.Sync(); err != nil {
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, path)
}

// SetContextWindow sets max messages to keep (default 20).
func (c *ConversationManager) SetContextWindow(n int) {
	if n < 2 {
		n = 2
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.window = n
}

// Append adds a message for user_id and returns context for LLM. Persists to JSONL.
func (c *ConversationManager) Append(userID, sessionID, role, content string) ([]llm.Message, error) {
	if c.memory != nil {
		if err := c.memory.SaveMemory(userID, sessionID, role, content); err != nil {
			return nil, err
		}
	}
	c.mu.Lock()
	msgs := c.sessions[userID]
	msgs = append(msgs, llm.Message{Role: role, Content: content})
	if len(msgs) > c.window {
		msgs = msgs[len(msgs)-c.window:]
	}
	c.sessions[userID] = msgs
	ctx := make([]llm.Message, len(msgs))
	copy(ctx, msgs)
	c.mu.Unlock()
	if err := c.persist(userID, msgs); err != nil {
		return ctx, fmt.Errorf("persist: %w", err)
	}
	return ctx, nil
}

// LoadContext loads history from file for user_id.
func (c *ConversationManager) LoadContext(userID, sessionID string) ([]llm.Message, error) {
	msgs := c.loadFromFile(c.convPath(userID))
	if len(msgs) == 0 && c.memory != nil {
		entries, err := c.memory.LoadHistory(userID, sessionID)
		if err != nil || len(entries) == 0 {
			return nil, err
		}
		msgs = make([]llm.Message, 0, len(entries))
		for _, e := range entries {
			msgs = append(msgs, llm.Message{Role: e.Role, Content: e.Content})
		}
	}
	if len(msgs) > c.window {
		msgs = msgs[len(msgs)-c.window:]
	}
	c.mu.Lock()
	c.sessions[userID] = msgs
	ctx := make([]llm.Message, len(msgs))
	copy(ctx, msgs)
	c.mu.Unlock()
	return ctx, nil
}

// GetContext returns current context for user without loading.
func (c *ConversationManager) GetContext(userID, sessionID string) []llm.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.sessions[userID]
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	return out
}
