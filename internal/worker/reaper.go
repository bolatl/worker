package worker

import (
	"context"
	"log"
	"time"

	"worker/internal/jobs"
)

type Reaper struct {
	repo    *jobs.Repository
	timeout time.Duration
}

func NewReaper(repo *jobs.Repository, timeout time.Duration) *Reaper {
	return &Reaper{repo: repo, timeout: timeout}
}

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
				log.Printf("reaper: requeued %d stuck jobs", n)
			}
		}
	}
}
