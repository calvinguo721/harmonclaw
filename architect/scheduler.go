// Package architect (scheduler) provides skill scheduling with priority, dependency, Ledger.
package architect

import (
	"context"
	"sort"
	"sync"
	"time"

	"harmonclaw/skills"
	"harmonclaw/viking"
)

const (
	PriorityLow    = 0
	PriorityNormal = 5
	PriorityHigh   = 10
	PriorityCritical = 15
)

// ScheduledTask represents a skill execution request.
type ScheduledTask struct {
	SkillID   string
	Input     skills.SkillInput
	Priority  int
	DependsOn []string
}

// Scheduler manages skill execution with priority, dependency chain, parallel no-dep, Ledger.
type Scheduler struct {
	pool   *WorkerPool
	guard  interface{ CheckSkill(string) (bool, string) }
	ledger viking.Ledger
	mu     sync.Mutex
	done   map[string]chan struct{}
}

// NewScheduler creates a scheduler.
func NewScheduler(pool *WorkerPool, guard interface{ CheckSkill(string) (bool, string) }, ledger viking.Ledger) *Scheduler {
	return &Scheduler{
		pool:   pool,
		guard:  guard,
		ledger: ledger,
		done:   make(map[string]chan struct{}),
	}
}

func (s *Scheduler) recordSchedule(taskID, skillID, result string) {
	if s.ledger == nil {
		return
	}
	s.ledger.Record(viking.LedgerEntry{
		OperatorID: "architect",
		ActionType: "schedule",
		Resource:   skillID,
		Result:     result,
		Timestamp:  time.Now().Format(time.RFC3339),
		ActionID:   taskID,
	})
}

// Schedule submits a task. Waits for deps, runs, writes Ledger.
func (s *Scheduler) Schedule(ctx context.Context, task ScheduledTask, exec func(skills.SkillInput) skills.SkillOutput) error {
	if task.Priority <= 0 {
		task.Priority = PriorityNormal
	}
	taskID := task.Input.TraceID
	if taskID == "" {
		taskID = time.Now().Format("20060102150405")
	}
	for _, dep := range task.DependsOn {
		s.mu.Lock()
		ch := s.done[dep]
		s.mu.Unlock()
		if ch != nil {
			select {
			case <-ch:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	doneCh := make(chan struct{})
	s.mu.Lock()
	s.done[task.SkillID] = doneCh
	s.mu.Unlock()
	defer func() {
		close(doneCh)
		s.mu.Lock()
		delete(s.done, task.SkillID)
		s.mu.Unlock()
	}()
	t := func() {
		out := exec(task.Input)
		if out.Status == "ok" {
			s.recordSchedule(taskID, task.SkillID, "success")
		} else {
			s.recordSchedule(taskID, task.SkillID, "fail")
		}
	}
	return s.pool.Submit(t)
}

// tasksByPriority sorts tasks: higher priority first, then by dependency order.
func tasksByPriority(tasks []ScheduledTask) []ScheduledTask {
	out := make([]ScheduledTask, len(tasks))
	copy(out, tasks)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Priority > out[j].Priority
	})
	return out
}

// ScheduleParallel submits tasks with no deps in parallel. Writes Ledger per task.
func (s *Scheduler) ScheduleParallel(ctx context.Context, tasks []ScheduledTask, exec func(string, skills.SkillInput) skills.SkillOutput) ([]skills.SkillOutput, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	results := make([]skills.SkillOutput, len(tasks))
	resultCh := make(chan struct {
		idx int
		out skills.SkillOutput
	}, len(tasks))
	for i, t := range tasks {
		idx, task := i, t
		if task.Priority <= 0 {
			task.Priority = PriorityNormal
		}
		fn := func() {
			out := exec(task.SkillID, task.Input)
			resultCh <- struct {
				idx int
				out skills.SkillOutput
			}{idx, out}
			if s.ledger != nil {
				result := "fail"
				if out.Status == "ok" {
					result = "success"
				}
				s.ledger.Record(viking.LedgerEntry{
					OperatorID: "architect",
					ActionType: "schedule_parallel",
					Resource:   task.SkillID,
					Result:     result,
					Timestamp:  time.Now().Format(time.RFC3339),
					ActionID:   task.Input.TraceID,
				})
			}
		}
		if err := s.pool.Submit(fn); err != nil {
			results[idx] = skills.SkillOutput{Status: "error", Error: err.Error()}
			continue
		}
	}
	for i := 0; i < len(tasks); i++ {
		select {
		case r := <-resultCh:
			results[r.idx] = r.out
		case <-ctx.Done():
			return results, ctx.Err()
		}
	}
	return results, nil
}
