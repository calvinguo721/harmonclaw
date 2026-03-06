// Package skills (sandbox) provides RunSandboxed with timeout and recover.
package skills

import (
	"context"
	"fmt"
	"time"
)

const defaultTimeout = 30 * time.Second

func RunSandboxed(ctx context.Context, traceID string, fn func() SkillOutput) SkillOutput {
	return RunSandboxedWithTimeout(ctx, traceID, defaultTimeout, fn)
}

func RunSandboxedWithTimeout(ctx context.Context, traceID string, timeout time.Duration, fn func() SkillOutput) SkillOutput {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultCh := make(chan SkillOutput, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- SkillOutput{
					TraceID: traceID,
					Status:  "error",
					Error:   fmt.Sprintf("panic: %v", r),
				}
			}
		}()
		resultCh <- fn()
	}()

	select {
	case out := <-resultCh:
		out.TraceID = traceID
		return out
	case <-ctx.Done():
		return SkillOutput{
			TraceID: traceID,
			Status:  "error",
			Error:   "timeout",
		}
	}
}
