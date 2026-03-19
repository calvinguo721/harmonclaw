// Package butler (memory) provides auto-summary every 10 turns and injects into next dialogue.
package butler

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"harmonclaw/llm"
	"harmonclaw/viking"
)

const (
	turnsBeforeSummary = 10
	summaryClass       = "internal"
)

// EngramWriter writes to Viking engrams.
type EngramWriter interface {
	WriteEngram(filename string, content []byte, classification string) error
}

// MemoryManager wraps conversation with auto-summary and context injection.
type MemoryManager struct {
	mu         sync.Mutex
	conv       *ConversationManager
	engramDir  string
	turnCount  map[string]int
	lastSummary map[string]string
}

// NewMemoryManager creates a manager. engramBaseDir is ~/.harmonclaw/viking.
func NewMemoryManager(conv *ConversationManager, engramBaseDir string) *MemoryManager {
	return &MemoryManager{
		conv:        conv,
		engramDir:   engramBaseDir,
		turnCount:   make(map[string]int),
		lastSummary: make(map[string]string),
	}
}

// Append adds message, may trigger auto-summary on assistant reply, returns context with injected summary.
func (m *MemoryManager) Append(userID, sessionID, role, content string) ([]llm.Message, error) {
	msgs, err := m.conv.Append(userID, sessionID, role, content)
	if err != nil {
		return nil, err
	}
	if role == "assistant" {
		m.mu.Lock()
		m.turnCount[userID]++
		turns := m.turnCount[userID]
		m.mu.Unlock()
			if turns%turnsBeforeSummary == 0 && len(msgs) >= 4 {
			summary := m.formatSummary(msgs)
			m.writeSummary(userID, summary)
			m.mu.Lock()
			m.lastSummary[userID] = summary
			m.mu.Unlock()
		}
	}
	injected := m.injectSummary(userID, msgs)
	return injected, nil
}

func (m *MemoryManager) formatSummary(msgs []llm.Message) string {
	var b strings.Builder
	start := 0
	if len(msgs) > 20 {
		start = len(msgs) - 20
	}
	for _, msg := range msgs[start:] {
		b.WriteString(msg.Role)
		b.WriteString(": ")
		b.WriteString(msg.Content)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func (m *MemoryManager) writeSummary(userID, summary string) {
	ts := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("%s_summary_%s.txt", userID, ts)
	path, err := viking.EngramPathWithBase(m.engramDir, filename)
	if err != nil {
		return
	}
	content := fmt.Sprintf("# source=system\n# classification=%s\n# type=conversation_summary\n\n%s",
		summaryClass, summary)
	viking.SafeWrite(path, []byte(content), summaryClass)
}

func (m *MemoryManager) injectSummary(userID string, msgs []llm.Message) []llm.Message {
	m.mu.Lock()
	sum := m.lastSummary[userID]
	m.mu.Unlock()
	if sum == "" {
		return msgs
	}
	injected := make([]llm.Message, 0, len(msgs)+1)
	injected = append(injected, llm.Message{
		Role:    "system",
		Content: "Conversation context:\n" + sum,
	})
	injected = append(injected, msgs...)
	return injected
}

// LoadContext delegates to conversation and injects summary.
func (m *MemoryManager) LoadContext(userID, sessionID string) ([]llm.Message, error) {
	msgs, err := m.conv.LoadContext(userID, sessionID)
	if err != nil {
		return nil, err
	}
	return m.injectSummary(userID, msgs), nil
}

// GetContext returns context with injected summary.
func (m *MemoryManager) GetContext(userID, sessionID string) []llm.Message {
	msgs := m.conv.GetContext(userID, sessionID)
	return m.injectSummary(userID, msgs)
}
