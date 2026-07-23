package service

import (
	"fmt"

	"github.com/arham/ai-second-brain/internal/config"
)

type Dependencies struct {
	HealthRepository HealthRepository
	UserRepository   UserRepository
	JWT              config.JWTConfig
}

type Container struct {
	Health HealthService
	Auth   AuthService
}

func NewContainer(deps Dependencies) (*Container, error) {
	if deps.HealthRepository == nil {
		return nil, fmt.Errorf("health repository is required")
	}
	if deps.UserRepository == nil {
		return nil, fmt.Errorf("user repository is required")
	}
	if len(deps.JWT.Secret) < 32 || deps.JWT.Issuer == "" || deps.JWT.AccessTokenTTL <= 0 {
		return nil, fmt.Errorf("valid JWT configuration is required")
	}

	container := &Container{
		Health: newHealthService(deps.HealthRepository),
		Auth:   newAuthService(deps.UserRepository, deps.JWT),
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
	if c.Auth == nil {
		return fmt.Errorf("auth service is required")
	}
	return nil
}
