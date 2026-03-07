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
	guard   sandbox.Guard
	ledger  viking.Ledger
	pool    *WorkerPool
	grantFn func(string, string) bool

	registry *SkillRegistry
	crons    *CronStore

	mu     sync.Mutex
	status string
}

func New(guard sandbox.Guard, ledger viking.Ledger) *Architect {
	a := &Architect{
		guard:   guard,
		ledger:  ledger,
		pool:    NewWorkerPool(),
		status:  "ok",
		registry: NewSkillRegistry(),
	}
	a.registry.SyncFromGlobal()
	if cs, err := NewCronStore("configs/crons.json"); err == nil {
		a.crons = cs
		a.crons.Start(func(job CronJob) {
			args := job.Args
			if args == nil {
				args = make(map[string]string)
			}
			a.ExecuteSkill(job.SkillID, skills.SkillInput{
				TraceID: "cron-" + job.ID,
				Text:    "",
				Args:    args,
			})
		})
	}
	return a
}

func (a *Architect) SetGrantFunc(fn func(string, string) bool) { a.grantFn = fn }
func (a *Architect) Pool() *WorkerPool                        { return a.pool }
func (a *Architect) Registry() *SkillRegistry                 { return a.registry }
func (a *Architect) Crons() *CronStore {
	if a == nil {
		return nil
	}
	return a.crons
}

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
	sk, ok := a.registry.Get(skillID)
	if !ok {
		sk, ok = skills.Registry[skillID]
	}
	if !ok {
		data, _ := json.Marshal(map[string]string{"status": check.Status})
		return skills.SkillOutput{
			TraceID: input.TraceID,
			Status:  "ok",
			Data:    data,
		}, nil
	}
	resultCh := make(chan skills.SkillOutput, 1)
	timeout := SkillTimeout(skillID)
	task := func() {
		out := skills.RunSandboxedWithTimeout(context.Background(), input.TraceID, timeout, func() skills.SkillOutput {
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

func (a *Architect) CheckSkill(skillID string) (bool, string) {
	return a.guard.CheckSkill(skillID)
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
