// Package architect (scheduler) provides skill scheduling with priority and dependency.
package architect

import (
	"context"
	"sync"

	"harmonclaw/skills"
)

const (
	PriorityLow    = 0
	PriorityNormal = 5
	PriorityHigh   = 10
)

// ScheduledTask represents a skill execution request.
type ScheduledTask struct {
	SkillID   string
	Input     skills.SkillInput
	Priority  int
	DependsOn []string // skill IDs that must complete first
}

// Scheduler manages skill execution with priority and dependency chain.
type Scheduler struct {
	pool   *WorkerPool
	guard  interface{ CheckSkill(string) (bool, string) }
	mu     sync.Mutex
	done   map[string]chan struct{}
}

// NewScheduler creates a scheduler.
func NewScheduler(pool *WorkerPool, guard interface{ CheckSkill(string) (bool, string) }) *Scheduler {
	return &Scheduler{
		pool:  pool,
		guard: guard,
		done:  make(map[string]chan struct{}),
	}
}

// Schedule submits a task. For dependency chain, waits for deps then runs.
func (s *Scheduler) Schedule(ctx context.Context, task ScheduledTask, exec func(skills.SkillInput) skills.SkillOutput) error {
	if task.Priority <= 0 {
		task.Priority = PriorityNormal
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
	t := func() {
		exec(task.Input)
	}
	return s.pool.Submit(t)
}

// ScheduleParallel submits multiple tasks via pool for parallel execution.
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
		fn := func() {
			out := exec(task.SkillID, task.Input)
			resultCh <- struct {
				idx int
				out skills.SkillOutput
			}{idx, out}
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
