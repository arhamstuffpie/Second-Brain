package repository

import "fmt"

type Container struct {
	Health HealthRepository
}

func NewContainer(db DBTX) (*Container, error) {
	baseRepository, err := newBase(db)
	if err != nil {
		return nil, err
	}

	container := &Container{
		Health: newHealthRepository(baseRepository),
	}
	if err := container.Validate(); err != nil {
		return nil, err
	}
	return container, nil
}

func (c *Container) Validate() error {
	if c == nil {
		return fmt.Errorf("repository container is required")
	}
	if c.Health == nil {
		return fmt.Errorf("health repository is required")
	}
	return nil
}
