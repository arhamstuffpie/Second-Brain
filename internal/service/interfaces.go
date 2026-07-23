package service

import (
	"context"
	"time"
)

type HealthRepository interface {
	Ping(ctx context.Context) error
}

type HealthService interface {
	Check(ctx context.Context) (Health, error)
}

type Health struct {
	Status    string    `json:"status"`
	Database  string    `json:"database"`
	CheckedAt time.Time `json:"checked_at"`
}
