package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"worker/internal/jobs"
)

type Processor struct {
	repo         *jobs.Repository
	workDuration time.Duration
}

func NewProcessor(repo *jobs.Repository, workDuration time.Duration) *Processor {
	return &Processor{repo: repo, workDuration: workDuration}
}

func (p *Processor) Handle(ctx context.Context, jobID string) (terminal bool, err error) {
	ok, job, err := p.repo.TryMarkProcessing(ctx, jobID)
	if err != nil {
		return false, err
	}
	if !ok {
		// already terminal -> idempotent no-op
		if job.Status == jobs.StatusDone || job.Status == jobs.StatusFailed {
			return true, nil
		}
		// If it was queued/processing and we didn't take it (rare), treat as non-terminal no-op.
		return false, nil
	}

	// Simulated work:
	// - If type is "fail" OR payload has {"fail":true}, we fail to test poison jobs.
	time.Sleep(p.workDuration)

	shouldFail := job.Type == "fail"
	if !shouldFail && len(job.Payload) > 0 {
		var m map[string]any
		_ = json.Unmarshal(job.Payload, &m)
		if v, ok := m["fail"].(bool); ok && v {
			shouldFail = true
		}
	}

	if shouldFail {
		term, _, _, recErr := p.repo.RecordFailure(ctx, jobID, "simulated failure")
		if recErr != nil {
			return false, recErr
		}
		return term, fmt.Errorf("job %s failed", jobID)
	}

	// Example deterministic result (hash payload)
	sum := sha256.Sum256(job.Payload)
	res := map[string]any{
		"sha256": hex.EncodeToString(sum[:]),
	}
	b, _ := json.Marshal(res)

	if err := p.repo.MarkDone(ctx, jobID, b); err != nil {
		return false, err
	}
	return true, nil
}
