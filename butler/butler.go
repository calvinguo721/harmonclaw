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

	mu     sync.Mutex
	status string
}

func New(provider llm.Provider, mem viking.Memory, ledger viking.Ledger) *Butler {
	return &Butler{
		llm:    provider,
		memory: mem,
		ledger: ledger,
		queue:  NewRealtimeQueue(),
		status: "ok",
	}
}

func (b *Butler) SetGrantFunc(fn func(string, string) bool) { b.grantFn = fn }
func (b *Butler) Queue() *RealtimeQueue                     { return b.queue }

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

	resp, err := b.llm.Chat(req)
	if err != nil {
		return resp, err
	}

	sessionID := fmt.Sprintf("%d", time.Now().UnixMilli())
	const user = "default"

	if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1]
		if err := b.memory.SaveMemory(user, sessionID, last.Role, last.Content); err != nil {
			log.Printf("butler: viking save user: %v", err)
		}
	}
	if err := b.memory.SaveMemory(user, sessionID, "assistant", resp.Content); err != nil {
		log.Printf("butler: viking save assistant: %v", err)
	}

	return resp, nil
}
