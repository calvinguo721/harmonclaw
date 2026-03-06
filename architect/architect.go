// Package architect manages skill execution and worker pool.
package architect

import (
	"log"
	"sync"
	"time"

	"harmonclaw/bus"
	"harmonclaw/sandbox"
	"harmonclaw/skills"
	"harmonclaw/viking"
)

type SkillResult struct {
	Allowed bool
	Verdict string
	Status  string
	Result  string
}

type Architect struct {
	guard  sandbox.Guard
	ledger viking.Ledger
	pool   *WorkerPool
	grantFn func(string, string) bool

	mu     sync.Mutex
	status string
}

func New(guard sandbox.Guard, ledger viking.Ledger) *Architect {
	return &Architect{
		guard:  guard,
		ledger: ledger,
		pool:   NewWorkerPool(),
		status: "ok",
	}
}

func (a *Architect) SetGrantFunc(fn func(string, string) bool) { a.grantFn = fn }
func (a *Architect) Pool() *WorkerPool                        { return a.pool }

func (a *Architect) Status() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

func (a *Architect) SetOK() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = "ok"
}

func (a *Architect) SetDegraded() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = "degraded"
}

func (a *Architect) Pulse() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		bus.Send(bus.Message{
			From:    bus.Architect,
			Type:    "pulse",
			Payload: a.Status(),
		})
	}
}

func (a *Architect) RegisterSkill(name string, skill skills.Skill) {
	skills.Register(skill)
}

func (a *Architect) HandleSkill(skillID string) SkillResult {
	allowed, verdict := a.guard.CheckSkill(skillID)

	if !allowed {
		log.Printf("architect: BLOCKED skill=%q", skillID)
		return SkillResult{Allowed: false, Verdict: verdict}
	}

	log.Printf("architect: APPROVED skill=%q", skillID)

	return SkillResult{
		Allowed: true,
		Verdict: verdict,
		Status:  "executed",
		Result:  "All systems nominal",
	}
}
