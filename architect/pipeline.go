// Package architect (pipeline) provides skill pipeline: A output -> B input.
package architect

import (
	"context"
	"encoding/json"

	"harmonclaw/skills"
)

// PipelineStage defines a step in the pipeline.
type PipelineStage struct {
	SkillID string
	Args    map[string]string
}

// Pipeline runs stages sequentially, passing output to next input.
type Pipeline struct {
	pool   *WorkerPool
	guard  interface{ CheckSkill(string) (bool, string) }
	stages []PipelineStage
}

// NewPipeline creates a pipeline.
func NewPipeline(pool *WorkerPool, guard interface{ CheckSkill(string) (bool, string) }, stages []PipelineStage) *Pipeline {
	return &Pipeline{
		pool:   pool,
		guard:  guard,
		stages: stages,
	}
}

// Run executes the pipeline. Each stage receives previous output as Text.
func (p *Pipeline) Run(ctx context.Context, traceID string, initialInput string, registry *SkillRegistry) (skills.SkillOutput, error) {
	input := skills.SkillInput{
		TraceID:   traceID,
		Text:      initialInput,
		Args:      make(map[string]string),
		LocalOnly: true,
	}
	var lastOut skills.SkillOutput
	for _, stage := range p.stages {
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
			break
		}
		resultCh := make(chan skills.SkillOutput, 1)
		task := func() {
			resultCh <- skills.RunSandboxed(ctx, traceID, func() skills.SkillOutput {
				return sk.Execute(input)
			})
		}
		if err := p.pool.Submit(task); err != nil {
			return skills.SkillOutput{
				TraceID: traceID,
				Status:  "error",
				Error:   err.Error(),
			}, err
		}
		lastOut = <-resultCh
		if lastOut.Status != "ok" {
			break
		}
		input.Text = string(lastOut.Data)
		if len(lastOut.Data) > 0 {
			var m map[string]string
			if json.Unmarshal(lastOut.Data, &m) == nil {
				for k, v := range m {
					input.Args[k] = v
				}
			}
		}
	}
	return lastOut, nil
}

