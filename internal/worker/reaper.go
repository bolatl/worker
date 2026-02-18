// Package worker provides job consumers, processors, and a reaper for stuck jobs.
package worker

import (
	"context"
	"log"
	"time"

	"worker/internal/jobs"
)

// Reaper periodically finds jobs stuck in processing and requeues them.
type Reaper struct {
	repo    *jobs.Repository
	timeout time.Duration
}

// NewReaper creates a Reaper that requeues jobs stuck in processing longer than timeout.
func NewReaper(repo *jobs.Repository, timeout time.Duration) *Reaper {
	return &Reaper{repo: repo, timeout: timeout}
}

// Run starts the reaper loop. It ticks every 10 seconds and requeues stuck jobs until ctx is canceled.
func (r *Reaper) Run(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := r.repo.RequeueStuckProcessing(ctx, r.timeout)
			if err != nil {
				log.Printf("reaper error: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("reaper: processed %d stuck jobs (requeued or failed)", n)
			}
		}
	}
}
