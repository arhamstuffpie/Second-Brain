package router

import (
	"fmt"
	"net/http"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/arham/ai-second-brain/internal/http/middleware"
	"github.com/arham/ai-second-brain/internal/http/response"
	"github.com/arham/ai-second-brain/internal/http/routes"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type Dependencies struct {
	Config     config.Config
	Logger     zerolog.Logger
	Handlers   *handler.Container
	Middleware *middleware.Container
}

func New(deps Dependencies) (*gin.Engine, error) {
	if deps.Handlers == nil || deps.Middleware == nil {
		return nil, fmt.Errorf("router dependencies are incomplete")
	}
	if deps.Config.Environment == "test" {
		gin.SetMode(gin.TestMode)
	} else {
		// Gin's debug route banner is unstructured. Application request and
		// startup logs are emitted through Zerolog instead.
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.HandleMethodNotAllowed = true
	if err := engine.SetTrustedProxies(deps.Config.HTTP.TrustedProxies); err != nil {
		return nil, fmt.Errorf("configure trusted proxies: %w", err)
	}
	engine.Use(
		middleware.RequestID(),
		middleware.RequestLogger(deps.Logger),
		middleware.Recovery(deps.Logger),
		middleware.CORS(deps.Config.CORS),
	)
	engine.NoRoute(func(c *gin.Context) {
		response.Error(c, http.StatusNotFound, "ROUTE_NOT_FOUND", "route was not found")
	})
	engine.NoMethod(func(c *gin.Context) {
		response.Error(c, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method is not allowed")
	})

	if err := routes.Register(engine, routes.Dependencies{
		Handlers: deps.Handlers, Middleware: deps.Middleware,
	}); err != nil {
		return nil, fmt.Errorf("register routes: %w", err)
	}
	return engine, nil
}
