package governor

import (
	"log"
	"sync"
	"time"

	"harmonclaw/viking"
)

const (
	heartbeatTimeout  = 30 * time.Second
	summarizeInterval = 1 * time.Hour
)

type watchTarget struct {
	name    string
	hb      <-chan time.Time
	restart func()
}

type Agent struct {
	ledger  viking.Ledger
	targets []watchTarget

	mu      sync.Mutex
	done    chan struct{}
	running bool
}

func New(ledger viking.Ledger) *Agent {
	return &Agent{ledger: ledger}
}

func (a *Agent) WatchAgent(name string, hb <-chan time.Time, restart func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.targets = append(a.targets, watchTarget{name, hb, restart})
}

func (a *Agent) Start() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return
	}
	a.done = make(chan struct{})
	a.running = true

	for _, t := range a.targets {
		go a.watchLoop(t)
	}
	go a.summarizeLoop()

	log.Printf("governor: online — monitoring %d agents", len(a.targets))
}

func (a *Agent) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return
	}
	close(a.done)
	a.running = false
	log.Println("governor: offline")
}

func (a *Agent) RequestGrant(core, peerID string) bool {
	if core == "governor" {
		log.Printf("governor: DENY grant %s -> %s (governor never goes online)", core, peerID)
		return false
	}
	log.Printf("governor: GRANT %s -> %s", core, peerID)
	return true
}

// --- heartbeat monitor ---

func (a *Agent) watchLoop(t watchTarget) {
	for {
		select {
		case <-t.hb:
			// agent is alive
		case <-time.After(heartbeatTimeout):
			log.Printf("governor: %s heartbeat TIMEOUT — attempting self-heal", t.name)
			t.restart()
		case <-a.done:
			return
		}
	}
}

// --- periodic summarization ---

func (a *Agent) summarizeLoop() {
	ticker := time.NewTicker(summarizeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.summarize()
		case <-a.done:
			return
		}
	}
}

func (a *Agent) summarize() {
	entries, err := a.ledger.Latest(50)
	if err != nil {
		log.Printf("governor: summarize error: %v", err)
		return
	}

	chatCount, skillCount, totalTokens := 0, 0, 0
	for _, e := range entries {
		totalTokens += e.Tokens
		if e.Action == "chat" {
			chatCount++
		} else {
			skillCount++
		}
	}

	log.Printf("governor: DIGEST — chats=%d skills=%d tokens=%d entries_scanned=%d",
		chatCount, skillCount, totalTokens, len(entries))
}
