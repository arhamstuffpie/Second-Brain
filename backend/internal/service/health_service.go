package service

import (
	"context"
	"time"
)

type healthService struct {
	repository HealthRepository
	now        func() time.Time
}

func newHealthService(repository HealthRepository) *healthService {
	return &healthService{repository: repository, now: time.Now}
}

func (s *healthService) Check(ctx context.Context) (Health, error) {
	if err := s.repository.Ping(ctx); err != nil {
		return Health{}, &UnavailableError{Cause: err}
	}

	return Health{
		Status:    "ok",
		Database:  "up",
		CheckedAt: s.now().UTC(),
	}, nil
}

var _ HealthService = (*healthService)(nil)
