package architect

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"harmonclaw/sandbox"
	"harmonclaw/skills"
	"harmonclaw/viking"
)

func TestScheduler_Priority(t *testing.T) {
	sorted := tasksByPriority([]ScheduledTask{
		{SkillID: "a", Priority: PriorityLow},
		{SkillID: "b", Priority: PriorityCritical},
		{SkillID: "c", Priority: PriorityNormal},
	})
	if sorted[0].SkillID != "b" || sorted[1].SkillID != "c" || sorted[2].SkillID != "a" {
		t.Errorf("priority sort: got %v", sorted)
	}
}

func TestScheduler_Schedule(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-sched-test")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	ledger, _ := viking.NewFileLedger(dir)
	defer ledger.Close()
	guard := sandbox.NewWhitelist()
	pool := NewWorkerPool()
	pool.Start()
	defer pool.Stop()
	sched := NewScheduler(pool, guard, ledger)
	ctx := context.Background()
	err := sched.Schedule(ctx, ScheduledTask{
		SkillID:  "test",
		Input:    skills.SkillInput{TraceID: "t1", Text: "x"},
		Priority: PriorityHigh,
	}, func(in skills.SkillInput) skills.SkillOutput {
		return skills.SkillOutput{Status: "ok", TraceID: in.TraceID}
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestScheduler_ScheduleParallel(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-sched-par")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	ledger, _ := viking.NewFileLedger(dir)
	defer ledger.Close()
	guard := sandbox.NewWhitelist()
	pool := NewWorkerPool()
	pool.Start()
	defer pool.Stop()
	sched := NewScheduler(pool, guard, ledger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results, err := sched.ScheduleParallel(ctx, []ScheduledTask{
		{SkillID: "a", Input: skills.SkillInput{TraceID: "t1"}, Priority: PriorityNormal},
		{SkillID: "b", Input: skills.SkillInput{TraceID: "t2"}, Priority: PriorityNormal},
	}, func(skillID string, in skills.SkillInput) skills.SkillOutput {
		return skills.SkillOutput{Status: "ok", TraceID: in.TraceID}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("ScheduleParallel: want 2, got %d", len(results))
	}
}
