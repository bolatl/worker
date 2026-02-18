package jobs

import "time"

// Job status constants.
const (
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusDone       = "done"
	StatusFailed     = "failed"
)

// Job represents a single background job with payload, result, and execution metadata.
type Job struct {
	ID      string
	Type    string
	Payload []byte // raw json
	Status  string

	Attempts    int
	MaxAttempts int

	Result    []byte
	LastError *string

	CreatedAt           time.Time
	UpdatedAt           time.Time
	ProcessingStartedAt *time.Time
}
