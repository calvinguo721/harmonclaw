package architect

import (
	"log"
	"sync"
	"time"

	"harmonclaw/sandbox"
	"harmonclaw/viking"
)

type SkillResult struct {
	Allowed bool
	Verdict string
	Status  string
	Result  string
}

type Agent struct {
	guard   sandbox.Guard
	ledger  viking.Ledger
	grantFn func(string, string) bool

	hb      chan time.Time
	done    chan struct{}
	mu      sync.Mutex
	running bool
}

func New(guard sandbox.Guard, ledger viking.Ledger) *Agent {
	return &Agent{
		guard:  guard,
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
	log.Println("architect: online")
}

func (a *Agent) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return
	}
	close(a.done)
	a.running = false
	log.Println("architect: offline")
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

func (a *Agent) HandleSkill(skillID string) SkillResult {
	allowed, verdict := a.guard.CheckSkill(skillID)

	if !allowed {
		log.Printf("architect: BLOCKED skill=%q", skillID)
		return SkillResult{Allowed: false, Verdict: verdict}
	}

	log.Printf("architect: APPROVED skill=%q", skillID)
	a.ledger.Record(viking.LedgerEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Role:      "system",
		Action:    "skill:" + skillID,
		Tokens:    0,
		Status:    "executed",
	})

	return SkillResult{
		Allowed: true,
		Verdict: verdict,
		Status:  "executed",
		Result:  "All systems nominal",
	}
}
