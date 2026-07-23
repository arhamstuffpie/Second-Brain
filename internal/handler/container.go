package handler

import (
	"fmt"

	"github.com/arham/ai-second-brain/internal/service"
)

type Dependencies struct {
	HealthService service.HealthService
	AuthService   service.AuthService
}

type Container struct {
	Health HealthHandler
	Auth   AuthHandler
}

func NewContainer(deps Dependencies) (*Container, error) {
	if deps.HealthService == nil {
		return nil, fmt.Errorf("health service is required")
	}
	if deps.AuthService == nil {
		return nil, fmt.Errorf("auth service is required")
	}

	container := &Container{
		Health: newHealthHandler(deps.HealthService),
		Auth:   newAuthHandler(deps.AuthService),
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
	if c.Auth == nil {
		return fmt.Errorf("auth handler is required")
	}
	return nil
}
