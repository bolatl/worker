package jobs

import (
	"context"
	"encoding/json"
)

type Publisher interface {
	PublishJob(ctx context.Context, jobID string) error
}

type Service struct {
	repo        *Repository
	publisher   Publisher
	maxAttempts int
}

func NewService(repo *Repository, publisher Publisher, maxAttempts int) *Service {
	return &Service{repo: repo, publisher: publisher, maxAttempts: maxAttempts}
}

func (s *Service) CreateJob(ctx context.Context, typ string, payload json.RawMessage) (string, error) {
	id, err := s.repo.Create(ctx, typ, payload, s.maxAttempts)
	if err != nil {
		return "", err
	}
	if err := s.publisher.PublishJob(ctx, id); err != nil {
		return "", err
	}
	return id, nil
}
