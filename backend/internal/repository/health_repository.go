package repository

import (
	"context"
	"fmt"
)

type HealthRepository interface {
	Ping(ctx context.Context) error
}

type healthRepository struct {
	*base
}

func newHealthRepository(base *base) *healthRepository {
	return &healthRepository{base: base}
}

func (r *healthRepository) Ping(ctx context.Context) error {
	result, err := r.queries.HealthCheck(ctx)
	if err != nil {
		return fmt.Errorf("health check query: %w", err)
	}
	if result != 1 {
		return fmt.Errorf("health check query returned unexpected result %d", result)
	}
	return nil
}

var _ HealthRepository = (*healthRepository)(nil)
