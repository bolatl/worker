package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, typ string, payload json.RawMessage, maxAttempts int) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO jobs(type, payload, status, attempts, max_attempts)
		VALUES ($1, $2, $3, 0, $4)
		RETURNING id::text
	`, typ, payload, StatusQueued, maxAttempts).Scan(&id)
	return id, err
}

func (r *Repository) Get(ctx context.Context, id string) (Job, error) {
	var j Job
	var payload, result []byte
	var lastErr *string
	var procAt *time.Time

	err := r.pool.QueryRow(ctx, `
		SELECT id::text, type, payload, status, attempts, max_attempts, result, last_error, processing_started_at, created_at, updated_at
		FROM jobs
		WHERE id = $1
	`, id).Scan(&j.ID, &j.Type, &payload, &j.Status, &j.Attempts, &j.MaxAttempts, &result, &lastErr, &procAt, &j.CreatedAt, &j.UpdatedAt)

	if err != nil {
		return Job{}, err
	}

	j.Payload = payload
	j.Result = result
	j.LastError = lastErr
	j.ProcessingStartedAt = procAt
	return j, nil
}

// TryMarkProcessing sets status to processing ONLY if job is not done/failed.
// Returns ok=false if it should be treated as no-op (idempotency).
func (r *Repository) TryMarkProcessing(ctx context.Context, id string) (ok bool, job Job, err error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, Job{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock row for safe transitions
	var payload, result []byte
	var lastErr *string
	var procAt *time.Time

	err = tx.QueryRow(ctx, `
		SELECT id::text, type, payload, status, attempts, max_attempts, result, last_error, processing_started_at, created_at, updated_at
		FROM jobs
		WHERE id = $1
		FOR UPDATE
	`, id).Scan(&job.ID, &job.Type, &payload, &job.Status, &job.Attempts, &job.MaxAttempts, &result, &lastErr, &procAt, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return false, Job{}, err
	}
	job.Payload = payload
	job.Result = result
	job.LastError = lastErr
	job.ProcessingStartedAt = procAt

	if job.Status == StatusDone || job.Status == StatusFailed {
		// already terminal -> idempotent no-op
		if err := tx.Commit(ctx); err != nil {
			return false, Job{}, err
		}
		return false, job, nil
	}

	if job.Status == StatusProcessing {
		// Another consumer already has the job. Avoid concurrent processing.
		// Let the caller decide how to retry (message will typically be requeued).
		if err := tx.Commit(ctx); err != nil {
			return false, Job{}, err
		}
		return false, job, nil
	}

	if job.Status != StatusQueued {
		// Unexpected state; be conservative and do nothing.
		if err := tx.Commit(ctx); err != nil {
			return false, Job{}, err
		}
		return false, job, nil
	}

	// Mark processing
	_, err = tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2, processing_started_at = now()
		WHERE id = $1
	`, id, StatusProcessing)
	if err != nil {
		return false, Job{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, Job{}, err
	}
	return true, job, nil
}

func (r *Repository) MarkDone(ctx context.Context, id string, result json.RawMessage) error {
	// Idempotency: only transition to done from queued/processing, and don't overwrite
	// terminal states (e.g., done/failed) with a different result.
	_, err := r.pool.Exec(ctx, `
		UPDATE jobs
		SET status = $2, result = $3, last_error = NULL, processing_started_at = NULL
		WHERE id = $1
		  AND status IN ($4, $5)
	`, id, StatusDone, result, StatusQueued, StatusProcessing)
	return err
}

// RecordFailure increments attempts and sets error.
// Returns terminal=true if job is now failed (poison job).
func (r *Repository) RecordFailure(ctx context.Context, id string, errMsg string) (terminal bool, attempts int, maxAttempts int, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, 0, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	err = tx.QueryRow(ctx, `
		SELECT status, attempts, max_attempts
		FROM jobs
		WHERE id = $1
		FOR UPDATE
	`, id).Scan(&status, &attempts, &maxAttempts)
	if err != nil {
		return false, 0, 0, err
	}

	if status == StatusDone || status == StatusFailed {
		// terminal already
		if err := tx.Commit(ctx); err != nil {
			return false, attempts, maxAttempts, err
		}
		return true, attempts, maxAttempts, nil
	}

	attempts++

	newStatus := StatusQueued
	terminal = false
	if attempts >= maxAttempts {
		newStatus = StatusFailed
		terminal = true
	}

	_, err = tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2, attempts = $3, last_error = $4, processing_started_at = NULL
		WHERE id = $1
	`, id, newStatus, attempts, errMsg)
	if err != nil {
		return false, 0, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, attempts, maxAttempts, err
	}
	return terminal, attempts, maxAttempts, nil
}

func (r *Repository) RequeueStuckProcessing(ctx context.Context, timeout time.Duration) (int64, error) {
	// Anything "processing" older than timeout becomes queued again.
	ct, err := r.pool.Exec(ctx, `
		UPDATE jobs
		SET status = $1, processing_started_at = NULL
		WHERE status = $2
		  AND processing_started_at IS NOT NULL
		  AND processing_started_at < (now() - $3::interval)
	`, StatusQueued, StatusProcessing, fmt.Sprintf("%f seconds", timeout.Seconds()))
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}
