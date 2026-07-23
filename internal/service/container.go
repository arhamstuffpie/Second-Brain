package service

import "fmt"

type Dependencies struct {
	HealthRepository HealthRepository
}

type Container struct {
	Health HealthService
}

func NewContainer(deps Dependencies) (*Container, error) {
	if deps.HealthRepository == nil {
		return nil, fmt.Errorf("health repository is required")
	}

	container := &Container{
		Health: newHealthService(deps.HealthRepository),
	}
	if err := container.Validate(); err != nil {
		return nil, err
	}
	return container, nil
}

func (c *Container) Validate() error {
	if c == nil {
		return fmt.Errorf("service container is required")
	}
	if c.Health == nil {
		return fmt.Errorf("health service is required")
	}
	return nil
}
