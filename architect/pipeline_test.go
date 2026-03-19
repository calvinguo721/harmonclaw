package architect

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"harmonclaw/sandbox"
	"harmonclaw/skills"
	"harmonclaw/viking"
)

type mockSkill struct {
	id string
}

func (m *mockSkill) Execute(in skills.SkillInput) skills.SkillOutput {
	return skills.SkillOutput{Status: "ok", TraceID: in.TraceID, Data: []byte(`{"out":"done"}`)}
}

func (m *mockSkill) GetIdentity() skills.SkillIdentity {
	return skills.SkillIdentity{ID: m.id, Version: "1.0", Core: "test"}
}

func TestPipeline_Run(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-pipe-test")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	ledger, _ := viking.NewFileLedger(dir)
	defer ledger.Close()
	guard := sandbox.NewWhitelist()
	pool := NewWorkerPool()
	pool.Start()
	defer pool.Stop()
	reg := NewSkillRegistry()
	reg.Register(&mockSkill{id: "mock"})
	pipe := NewPipeline(pool, guard, ledger, []PipelineStage{
		{SkillID: "mock", Args: map[string]string{"x": "1"}},
	})
	ctx := context.Background()
	out, err := pipe.Run(ctx, "trace1", "input", reg)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" {
		t.Errorf("pipeline: want ok, got %s", out.Status)
	}
}

func TestPipeline_SkillNotFound(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-pipe-nf")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	ledger, _ := viking.NewFileLedger(dir)
	defer ledger.Close()
	guard := sandbox.NewWhitelist()
	pool := NewWorkerPool()
	pool.Start()
	defer pool.Stop()
	reg := NewSkillRegistry()
	pipe := NewPipeline(pool, guard, ledger, []PipelineStage{
		{SkillID: "nonexistent"},
	})
	ctx := context.Background()
	out, err := pipe.Run(ctx, "trace2", "", reg)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "error" {
		t.Errorf("skill not found: want error, got %s", out.Status)
	}
}
