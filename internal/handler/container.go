package handler

import (
	"fmt"

	"github.com/arham/ai-second-brain/internal/service"
)

type Dependencies struct {
	HealthService service.HealthService
}

type Container struct {
	Health HealthHandler
}

func NewContainer(deps Dependencies) (*Container, error) {
	if deps.HealthService == nil {
		return nil, fmt.Errorf("health service is required")
	}

	container := &Container{
		Health: newHealthHandler(deps.HealthService),
	}
	if err := container.Validate(); err != nil {
		return nil, err
	}
	return container, nil
}

func (c *Container) Validate() error {
	if c == nil {
		return fmt.Errorf("handler container is required")
	}
	if c.Health == nil {
		return fmt.Errorf("health handler is required")
	}
	return nil
}
