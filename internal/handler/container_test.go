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

func TestNewContainerRequiresServices(t *testing.T) {
	if _, err := NewContainer(Dependencies{}); err == nil {
		t.Fatal("NewContainer() error = nil, want error")
	}
}

func TestNewContainerPopulatesDependencies(t *testing.T) {
	container, err := NewContainer(Dependencies{HealthService: stubHealthService{}})
	if err != nil {
		t.Fatalf("NewContainer() error = %v", err)
	}
	if container == nil || container.Health == nil {
		t.Fatal("handler container has nil required dependency")
	}
}
