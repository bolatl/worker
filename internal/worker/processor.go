package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"worker/internal/jobs"
)

// Processor handles individual job execution: marks processing, does work, marks done or failed.
type Processor struct {
	repo         *jobs.Repository
	workDuration time.Duration
}

// NewProcessor creates a Processor with the given repository and simulated work duration.
func NewProcessor(repo *jobs.Repository, workDuration time.Duration) *Processor {
	return &Processor{repo: repo, workDuration: workDuration}
}

// Handle processes a single job by ID. Returns terminal=true when the job reaches a final state (done/failed).
func (p *Processor) Handle(ctx context.Context, jobID string) (terminal bool, err error) {
	ok, job, err := p.repo.TryMarkProcessing(ctx, jobID)
	if err != nil {
		return false, fmt.Errorf("failed to mark job processing: %w", err)
	}

	if !ok {
		// already terminal -> idempotent no-op
		if job.Status == jobs.StatusDone || job.Status == jobs.StatusFailed {
			log.Printf("Worker: Job %s already in terminal state %s, treating as no-op", jobID, job.Status)
			return true, nil
		}
		// Queued/processing but we didn't take it: another worker has it, or reaper will reset stuck jobs.
		// No log here to avoid spam when worker crash leaves job in processing until reaper runs.
		return false, nil
	}

	log.Printf("Worker: Handling job job_id=%s", jobID)

	// Simulated work:
	// - If type is "fail" OR payload has {"fail":true}, we fail to test poison jobs.
	log.Printf("Worker: Job %s marked as processing, simulating work for %s", jobID, p.workDuration)
	time.Sleep(p.workDuration)

	shouldFail := job.Type == "fail"
	if !shouldFail && len(job.Payload) > 0 {
		var m map[string]any
		_ = json.Unmarshal(job.Payload, &m)
		log.Printf("Worker: Job %s payload decoded as %v", jobID, m)
		if v, ok := m["fail"].(bool); ok && v {
			shouldFail = true
		}
	}

	if shouldFail {
		log.Printf("Worker: Job %s is set to fail, recording failure", jobID)
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
		return false, fmt.Errorf("failed to mark job done: %w", err)
	}
	log.Printf("Worker: Job %s completed successfully with result %s", jobID, string(b))
	return true, nil
}
