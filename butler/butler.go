package butler

import (
	"fmt"
	"log"
	"sync"
	"time"

	"harmonclaw/llm"
	"harmonclaw/viking"
)

type Agent struct {
	llm     llm.Provider
	memory  viking.Memory
	ledger  viking.Ledger
	grantFn func(string, string) bool

	hb      chan time.Time
	done    chan struct{}
	mu      sync.Mutex
	running bool
}

func New(provider llm.Provider, mem viking.Memory, ledger viking.Ledger) *Agent {
	return &Agent{
		llm:    provider,
		memory: mem,
		ledger: ledger,
		hb:     make(chan time.Time, 1),
	}
}

func (a *Agent) SetGrantFunc(fn func(string, string) bool) { a.grantFn = fn }
func (a *Agent) Heartbeat() <-chan time.Time                { return a.hb }

func (a *Agent) Status() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return "ok"
	}
	return "degraded"
}

func (a *Agent) Start() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return
	}
	a.done = make(chan struct{})
	a.running = true
	go a.pulse()
	log.Println("butler: online")
}

func (a *Agent) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return
	}
	close(a.done)
	a.running = false
	log.Println("butler: offline")
}

func (a *Agent) pulse() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			select {
			case a.hb <- time.Now():
			default:
			}
		case <-a.done:
			return
		}
	}
}

func (a *Agent) HandleChat(req llm.Request) (llm.Response, error) {
	if a.grantFn != nil && !a.grantFn("butler", "deepseek-api") {
		return llm.Response{}, fmt.Errorf("grant denied: butler -> deepseek-api")
	}

	resp, err := a.llm.Chat(req)
	if err != nil {
		return resp, err
	}

	sessionID := fmt.Sprintf("%d", time.Now().UnixMilli())
	const user = "default"

	if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1]
		if err := a.memory.SaveMemory(user, sessionID, last.Role, last.Content); err != nil {
			log.Printf("butler: viking save user: %v", err)
		}
	}
	if err := a.memory.SaveMemory(user, sessionID, "assistant", resp.Content); err != nil {
		log.Printf("butler: viking save assistant: %v", err)
	}

	a.ledger.Record(viking.LedgerEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Role:      "assistant",
		Action:    "chat",
		Tokens:    len(resp.Content) / 4,
		Status:    "ok",
	})

	return resp, nil
}
