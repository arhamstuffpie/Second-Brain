package middleware

import (
	"testing"

	"github.com/arham/ai-second-brain/internal/config"
)

func TestNewContainerPopulatesDependencies(t *testing.T) {
	container, err := NewContainer(config.Config{JWT: config.JWTConfig{
		Secret: "01234567890123456789012345678901",
		Issuer: "test-issuer",
	}})
	if err != nil {
		t.Fatalf("NewContainer() error = %v", err)
	}
	if container == nil || container.JWT == nil {
		t.Fatal("middleware container has nil required dependency")
	}
}
