package middleware

import (
	"fmt"

	"github.com/arham/ai-second-brain/internal/config"
)

type Container struct {
	JWT JWTAuthenticator
}

func NewContainer(cfg config.Config) (*Container, error) {
	authenticator, err := NewJWTAuthenticator(cfg.JWT)
	if err != nil {
		return nil, err
	}
	container := &Container{JWT: authenticator}
	if err := container.Validate(); err != nil {
		return nil, err
	}
	return container, nil
}

func (c *Container) Validate() error {
	if c == nil {
		return fmt.Errorf("middleware container is required")
	}
	if c.JWT == nil {
		return fmt.Errorf("JWT authenticator is required")
	}
	return nil
}
