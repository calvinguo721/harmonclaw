// Package butler handles chat and user interaction.
package butler

import (
	"fmt"
	"log"
	"sync"
	"time"

	"harmonclaw/bus"
	"harmonclaw/llm"
	"harmonclaw/viking"
)

type Butler struct {
	llm     llm.Provider
	memory  viking.Memory
	ledger  viking.Ledger
	queue   *RealtimeQueue
	grantFn func(string, string) bool

	conv    *ConversationManager
	memMgr  *MemoryManager
	persona *PersonaStore

	mu     sync.Mutex
	status string
}

func New(provider llm.Provider, mem viking.Memory, ledger viking.Ledger) *Butler {
	return NewWithOpts(provider, mem, ledger, "", "")
}

func NewWithOpts(provider llm.Provider, mem viking.Memory, ledger viking.Ledger, vikingBaseDir, personaPath string) *Butler {
	b := &Butler{
		llm:    provider,
		memory: mem,
		ledger: ledger,
		queue:  NewRealtimeQueue(),
		status: "ok",
	}
	if vikingBaseDir != "" {
		memHist, _ := mem.(viking.MemoryWithHistory)
		b.conv = NewConversationManager(memHist, vikingBaseDir)
		b.memMgr = NewMemoryManager(b.conv, vikingBaseDir)
	}
	if personaPath != "" {
		if ps, err := NewPersonaStore(personaPath); err == nil {
			b.persona = ps
		}
	}
	return b
}

func (b *Butler) SetGrantFunc(fn func(string, string) bool) { b.grantFn = fn }
func (b *Butler) Queue() *RealtimeQueue                     { return b.queue }
func (b *Butler) Persona() *PersonaStore                     { return b.persona }
func (b *Butler) MemoryManager() *MemoryManager              { return b.memMgr }

func (b *Butler) Status() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status
}

func (b *Butler) SetOK() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status = "ok"
}

func (b *Butler) SetDegraded() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status = "degraded"
}

func (b *Butler) Pulse() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		bus.Send(bus.Message{
			From:    bus.Butler,
			Type:    "pulse",
			Payload: b.Status(),
		})
	}
}

func (b *Butler) HandleChat(req llm.Request) (llm.Response, error) {
	if b.grantFn != nil && !b.grantFn("butler", "deepseek-api") {
		return llm.Response{}, fmt.Errorf("grant denied: butler -> deepseek-api")
	}
	sessionID := fmt.Sprintf("%d", time.Now().UnixMilli())
	user := "default"
	messages := req.Messages
	if b.memMgr != nil {
		if len(req.Messages) > 0 {
			last := req.Messages[len(req.Messages)-1]
			var err error
			messages, err = b.memMgr.Append(user, sessionID, last.Role, last.Content)
			if err != nil {
				log.Printf("butler: memory append: %v", err)
			}
		} else {
			messages, _ = b.memMgr.LoadContext(user, sessionID)
		}
		if b.persona != nil {
			if pc, ok := b.persona.Get(""); ok {
				messages = prependSystem(messages, pc.SystemPrompt)
			}
		}
	} else if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1]
		b.memory.SaveMemory(user, sessionID, last.Role, last.Content)
	}
	req.Messages = messages
	resp, err := b.llm.Chat(req)
	if err != nil {
		return resp, err
	}
	if b.memMgr != nil {
		b.memMgr.Append(user, sessionID, "assistant", resp.Content)
	} else {
		b.memory.SaveMemory(user, sessionID, "assistant", resp.Content)
	}
	return resp, nil
}

func prependSystem(msgs []llm.Message, system string) []llm.Message {
	if system == "" {
		return msgs
	}
	out := make([]llm.Message, 0, len(msgs)+1)
	out = append(out, llm.Message{Role: "system", Content: system})
	out = append(out, msgs...)
	return out
}

// HandleChatStream returns a channel of content chunks and sessionID for streaming.
func (b *Butler) HandleChatStream(req llm.Request) (ch <-chan string, sessionID string, err error) {
	if b.grantFn != nil && !b.grantFn("butler", "deepseek-api") {
		return nil, "", fmt.Errorf("grant denied: butler -> deepseek-api")
	}
	sessionID = fmt.Sprintf("%d", time.Now().UnixMilli())
	user := "default"
	messages := req.Messages
	if b.memMgr != nil {
		if len(req.Messages) > 0 {
			last := req.Messages[len(req.Messages)-1]
			var e error
			messages, e = b.memMgr.Append(user, sessionID, last.Role, last.Content)
			if e != nil {
				log.Printf("butler: memory append: %v", e)
			}
		} else {
			messages, _ = b.memMgr.LoadContext(user, sessionID)
		}
		if b.persona != nil {
			if pc, ok := b.persona.Get(""); ok {
				messages = prependSystem(messages, pc.SystemPrompt)
			}
		}
		req.Messages = messages
	} else if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1]
		b.memory.SaveMemory(user, sessionID, last.Role, last.Content)
	}
	ch, err = b.llm.ChatStream(req)
	return ch, sessionID, err
}

// SaveStreamedResponse saves the full assistant response after streaming completes.
func (b *Butler) SaveStreamedResponse(user, sessionID, content string) {
	if b.memMgr != nil {
		b.memMgr.Append(user, sessionID, "assistant", content)
	} else if err := b.memory.SaveMemory(user, sessionID, "assistant", content); err != nil {
		log.Printf("butler: viking save assistant: %v", err)
	}
}
