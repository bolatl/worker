// Package jobs provides job persistence, service logic, and domain types.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles job persistence in Postgres
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a Repository using the given connection pool
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a new job with status queued and returns its UUID as a string.
func (r *Repository) Create(ctx context.Context, typ string, payload json.RawMessage, maxAttempts int) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO jobs(type, payload, status, attempts, max_attempts)
		VALUES ($1, $2, $3, 0, $4)
		RETURNING id::text
	`, typ, payload, StatusQueued, maxAttempts).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("failed to insert job: %w", err)
	}
	return id, nil
}

// Get fetches a job by ID. Returns pgx.ErrNoRows if not found.
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
		return Job{}, fmt.Errorf("failed to fetch job: %w", err)
	}

	j.Payload = payload
	j.Result = result
	j.LastError = lastErr
	j.ProcessingStartedAt = procAt
	return j, nil
}

// TryMarkProcessing atomically sets status to processing if the job is queued.
// Returns ok=false if already done/failed/processing (idempotent no-op).
func (r *Repository) TryMarkProcessing(ctx context.Context, id string) (ok bool, job Job, err error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, Job{}, fmt.Errorf("failed to begin transaction: %w", err)
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
		return false, Job{}, fmt.Errorf("failed to fetch job for update: %w", err)
	}
	job.Payload = payload
	job.Result = result
	job.LastError = lastErr
	job.ProcessingStartedAt = procAt

	if job.Status == StatusDone || job.Status == StatusFailed {
		// already terminal -> idempotent no-op
		if err := tx.Commit(ctx); err != nil {
			return false, Job{}, fmt.Errorf("failed to commit transaction: %w", err)
		}
		return false, job, nil
	}

	if job.Status == StatusProcessing {
		// Another consumer already has the job. Avoid concurrent processing.
		// Let the caller decide how to retry (message will typically be requeued).
		if err := tx.Commit(ctx); err != nil {
			return false, Job{}, fmt.Errorf("failed to commit transaction: %w", err)
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
		return false, Job{}, fmt.Errorf("failed to mark job as processing: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, Job{}, fmt.Errorf("failed to commit transaction: %w", err)
	}
	return true, job, nil
}

// MarkDone sets job status to done and stores the result. Idempotent for queued/processing only.
func (r *Repository) MarkDone(ctx context.Context, id string, result json.RawMessage) error {
	// Idempotency: only transition to done from queued/processing, and don't overwrite
	// terminal states (e.g., done/failed) with a different result.
	_, err := r.pool.Exec(ctx, `
		UPDATE jobs
		SET status = $2, result = $3, last_error = NULL, processing_started_at = NULL
		WHERE id = $1
		  AND status IN ($4, $5)
	`, id, StatusDone, result, StatusQueued, StatusProcessing)
	if err != nil {
		return fmt.Errorf("failed to mark job as done: %w", err)
	}
	return nil
}

// RecordFailure increments attempts, sets last_error, and requeues or marks failed.
// Returns terminal=true when attempts >= max_attempts (poison job).
func (r *Repository) RecordFailure(ctx context.Context, id string, errMsg string) (terminal bool, attempts int, maxAttempts int, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, 0, 0, fmt.Errorf("failed to begin transaction: %w", err)
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
		return false, 0, 0, fmt.Errorf("failed to fetch job for update: %w", err)
	}

	if status == StatusDone || status == StatusFailed {
		// terminal already
		if err := tx.Commit(ctx); err != nil {
			return false, attempts, maxAttempts, fmt.Errorf("failed to commit transaction: %w", err)
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
		return false, 0, 0, fmt.Errorf("failed to update job failure: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, attempts, maxAttempts, fmt.Errorf("failed to commit transaction: %w", err)
	}
	return terminal, attempts, maxAttempts, nil
}

// RequeueStuckProcessing resets jobs stuck in processing longer than timeout.
// Increments attempts. If attempts >= max_attempts, marks as failed; otherwise requeues.
// Returns the number of jobs processed (requeued or failed).
func (r *Repository) RequeueStuckProcessing(ctx context.Context, timeout time.Duration) (int64, error) {
	ct, err := r.pool.Exec(ctx, `
		UPDATE jobs
		SET
		  attempts = attempts + 1,
		  status = CASE WHEN attempts + 1 >= max_attempts THEN $1 ELSE $2 END,
		  last_error = CASE WHEN attempts + 1 >= max_attempts THEN 'stuck in processing, exceeded max requeue attempts' ELSE last_error END,
		  processing_started_at = NULL
		WHERE status = $3
		  AND processing_started_at IS NOT NULL
		  AND processing_started_at < (now() - $4::interval)
	`, StatusFailed, StatusQueued, StatusProcessing, fmt.Sprintf("%f seconds", timeout.Seconds()))
	if err != nil {
		return 0, fmt.Errorf("failed to requeue stuck processing jobs: %w", err)
	}
	return ct.RowsAffected(), nil
}
