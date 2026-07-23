package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

type stubHealthRepository struct {
	err error
}

func (s stubHealthRepository) Ping(context.Context) error {
	return s.err
}

type stubUserRepository struct{}

func (stubUserRepository) Create(context.Context, string, string) (StoredUser, bool, error) {
	return StoredUser{}, true, nil
}

func (stubUserRepository) FindByEmail(context.Context, string) (StoredUser, bool, error) {
	return StoredUser{}, true, nil
}

func testJWTConfig() config.JWTConfig {
	return config.JWTConfig{
		Secret:         "01234567890123456789012345678901",
		Issuer:         "test-issuer",
		AccessTokenTTL: time.Hour,
	}
}

func TestNewContainerRequiresRepositories(t *testing.T) {
	if _, err := NewContainer(Dependencies{}); err == nil {
		t.Fatal("NewContainer() error = nil, want error")
	}
}

func TestNewContainerPopulatesDependencies(t *testing.T) {
	container, err := NewContainer(Dependencies{
		HealthRepository: stubHealthRepository{},
		UserRepository:   stubUserRepository{},
		JWT:              testJWTConfig(),
	})
	if err != nil {
		t.Fatalf("NewContainer() error = %v", err)
	}
	if container == nil || container.Health == nil || container.Auth == nil {
		t.Fatal("service container has nil required dependency")
	}
}

func TestHealthServiceWrapsDependencyFailure(t *testing.T) {
	wantErr := errors.New("database down")
	service := newHealthService(stubHealthRepository{err: wantErr})

	_, err := service.Check(context.Background())
	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("Check() error = %v, want UnavailableError", err)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("Check() error does not wrap repository error")
	}
}

func TestHealthServiceReturnsHealthyState(t *testing.T) {
	service := newHealthService(stubHealthRepository{})
	health, err := service.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if health.Status != "ok" || health.Database != "up" || health.CheckedAt.IsZero() {
		t.Fatalf("Check() = %+v, want healthy state", health)
	}
}
