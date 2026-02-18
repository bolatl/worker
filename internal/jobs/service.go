package jobs

import (
	"context"
	"encoding/json"
	"fmt"
)

// Publisher publishes job IDs to a message queue (e.g., RabbitMQ).
type Publisher interface {
	PublishJob(ctx context.Context, jobID string) error
}

// Service orchestrates job creation and publishing.
type Service struct {
	repo        *Repository
	publisher   Publisher
	maxAttempts int
}

// NewService creates a Service with the given repository, publisher, and max retry attempts.
func NewService(repo *Repository, publisher Publisher, maxAttempts int) *Service {
	return &Service{repo: repo, publisher: publisher, maxAttempts: maxAttempts}
}

// CreateJob persists a job and publishes it to the queue. Returns the job ID.
func (s *Service) CreateJob(ctx context.Context, typ string, payload json.RawMessage) (string, error) {
	id, err := s.repo.Create(ctx, typ, payload, s.maxAttempts)
	if err != nil {
		return "", fmt.Errorf("failed to create job: %w", err)
	}
	if err := s.publisher.PublishJob(ctx, id); err != nil {
		return "", fmt.Errorf("failed to publish job: %w", err)
	}
	return id, nil
}
