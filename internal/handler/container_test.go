package handler

import (
	"context"
	"testing"

	"github.com/arham/ai-second-brain/internal/service"
)

type stubHealthService struct{}

func (stubHealthService) Check(context.Context) (service.Health, error) {
	return service.Health{}, nil
}

type stubAuthService struct{}

func (stubAuthService) Signup(context.Context, string, string) (service.AuthResult, error) {
	return service.AuthResult{}, nil
}

func (stubAuthService) Login(context.Context, string, string) (service.AuthResult, error) {
	return service.AuthResult{}, nil
}

func TestNewContainerRequiresServices(t *testing.T) {
	if _, err := NewContainer(Dependencies{}); err == nil {
		t.Fatal("NewContainer() error = nil, want error")
	}
}

func TestNewContainerPopulatesDependencies(t *testing.T) {
	container, err := NewContainer(Dependencies{
		HealthService: stubHealthService{},
		AuthService:   stubAuthService{},
	})
	if err != nil {
		t.Fatalf("NewContainer() error = %v", err)
	}
	if container == nil || container.Health == nil || container.Auth == nil {
		t.Fatal("handler container has nil required dependency")
	}
}
