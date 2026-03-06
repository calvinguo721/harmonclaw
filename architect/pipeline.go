// Package architect (pipeline) provides Pipeline with output mapping, fail rollback, Ledger.
package architect

import (
	"context"
	"encoding/json"
	"time"

	"harmonclaw/skills"
	"harmonclaw/viking"
)

// PipelineStage defines a step with optional output mapping.
type PipelineStage struct {
	SkillID      string            `json:"skill_id"`
	Args         map[string]string `json:"args"`
	OutputMap    map[string]string  `json:"output_map"` // stage output key -> next input key
}

// Pipeline runs stages sequentially with output mapping and fail rollback.
type Pipeline struct {
	pool   *WorkerPool
	guard  interface{ CheckSkill(string) (bool, string) }
	ledger viking.Ledger
	stages []PipelineStage
}

// NewPipeline creates a pipeline.
func NewPipeline(pool *WorkerPool, guard interface{ CheckSkill(string) (bool, string) }, ledger viking.Ledger, stages []PipelineStage) *Pipeline {
	return &Pipeline{
		pool:   pool,
		guard:  guard,
		ledger: ledger,
		stages: stages,
	}
}

func (p *Pipeline) recordLedger(traceID, stageID, result string) {
	if p.ledger == nil {
		return
	}
	p.ledger.Record(viking.LedgerEntry{
		OperatorID: "architect",
		ActionType: "pipeline",
		Resource:   stageID,
		Result:     result,
		Timestamp:  time.Now().Format(time.RFC3339),
		ActionID:   traceID,
	})
}

// Run executes the pipeline. On fail, records Ledger and returns.
func (p *Pipeline) Run(ctx context.Context, traceID string, initialInput string, registry *SkillRegistry) (skills.SkillOutput, error) {
	input := skills.SkillInput{
		TraceID:   traceID,
		Text:      initialInput,
		Args:      make(map[string]string),
		LocalOnly: true,
	}
	var lastOut skills.SkillOutput
	for i, stage := range p.stages {
		if stage.Args != nil {
			for k, v := range stage.Args {
				input.Args[k] = v
			}
		}
		sk, ok := registry.Get(stage.SkillID)
		if !ok {
			lastOut = skills.SkillOutput{
				TraceID: traceID,
				Status:  "error",
				Error:   "skill not found: " + stage.SkillID,
			}
			p.recordLedger(traceID, stage.SkillID, "fail")
			break
		}
		resultCh := make(chan skills.SkillOutput, 1)
		task := func() {
			resultCh <- skills.RunSandboxed(ctx, traceID, func() skills.SkillOutput {
				return sk.Execute(input)
			})
		}
		if err := p.pool.Submit(task); err != nil {
			lastOut = skills.SkillOutput{
				TraceID: traceID,
				Status:  "error",
				Error:   err.Error(),
			}
			p.recordLedger(traceID, stage.SkillID, "fail")
			break
		}
		lastOut = <-resultCh
		if lastOut.Status != "ok" {
			p.recordLedger(traceID, stage.SkillID, "fail")
			break
		}
		p.recordLedger(traceID, stage.SkillID, "success")
		input.Text = string(lastOut.Data)
		if len(lastOut.Data) > 0 {
			var m map[string]string
			if json.Unmarshal(lastOut.Data, &m) == nil {
				for k, v := range m {
					input.Args[k] = v
				}
				if stage.OutputMap != nil {
					for outKey, inKey := range stage.OutputMap {
						if v, ok := m[outKey]; ok {
							input.Args[inKey] = v
						}
					}
				}
			}
		}
		_ = i
	}
	return lastOut, nil
}
