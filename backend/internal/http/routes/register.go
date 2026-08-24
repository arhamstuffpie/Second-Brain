package routes

import (
	"fmt"

	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/arham/ai-second-brain/internal/http/middleware"
	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	Handlers   *handler.Container
	Middleware *middleware.Container
}

// Register keeps health public for orchestrators and prepares an authenticated
// API group for feature routes. Feature registrars receive dependencies rather
// than constructing them.
func Register(engine *gin.Engine, deps Dependencies) error {
	if engine == nil {
		return fmt.Errorf("gin engine is required")
	}
	if deps.Handlers == nil {
		return fmt.Errorf("handler container is required")
	}
	if err := deps.Handlers.Validate(); err != nil {
		return err
	}
	if deps.Middleware == nil {
		return fmt.Errorf("middleware container is required")
	}
	if err := deps.Middleware.Validate(); err != nil {
		return err
	}

	registerHealth(engine, deps.Handlers.Health)

	auth := engine.Group("/api/v1/auth")
	registerAuth(auth, deps.Handlers.Auth)

	protected := engine.Group("/api/v1")
	protected.Use(deps.Middleware.JWT.Handle())
	registerSecure(protected, deps.Handlers.Auth)
	registerModelProfiles(protected, deps.Handlers.Models)
	registerVoice(protected, deps.Handlers.Voice)
	registerVideo(protected, deps.Handlers.Video)
	registerPeople(protected, deps.Handlers.People)
	if deps.Handlers.Debug != nil {
		registerPipelineDebug(protected, deps.Handlers.Debug)
	}

	return nil
}
