// Package architect manages skill execution and worker pool.
package architect

import (
	"context"
	"encoding/json"
	"log"
	"runtime"
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
		payload := map[string]any{
			"status":     a.Status(),
			"goroutines": runtime.NumGoroutine(),
			"queue_len":  a.pool.QueueLen(),
		}
		bus.Send(bus.Message{
			From:    bus.Architect,
			Type:    "pulse",
			Payload: payload,
		})
	}
}

// ExecuteSkill runs a skill via WorkerPool and RunSandboxed.
func (a *Architect) ExecuteSkill(skillID string, input skills.SkillInput) (skills.SkillOutput, error) {
	check := a.HandleSkill(skillID)
	if !check.Allowed {
		return skills.SkillOutput{
			TraceID: input.TraceID,
			Status:  "error",
			Error:   check.Verdict,
		}, nil
	}
	sk, ok := skills.Registry[skillID]
	if !ok {
		data, _ := json.Marshal(map[string]string{"status": check.Status})
		return skills.SkillOutput{
			TraceID: input.TraceID,
			Status:  "ok",
			Data:    data,
		}, nil
	}
	resultCh := make(chan skills.SkillOutput, 1)
	task := func() {
		out := skills.RunSandboxed(context.Background(), input.TraceID, func() skills.SkillOutput {
			return sk.Execute(input)
		})
		resultCh <- out
	}
	if err := a.pool.Submit(task); err != nil {
		return skills.SkillOutput{
			TraceID: input.TraceID,
			Status:  "error",
			Error:   err.Error(),
		}, err
	}
	return <-resultCh, nil
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
