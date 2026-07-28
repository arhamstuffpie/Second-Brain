package routes

import (
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerHealth(router gin.IRoutes, health handler.HealthHandler) {
	// GET /health
	// Public readiness check. Takes no parameters and returns 200 with a
	// service.Health payload, or 503 when the database cannot be reached.
	router.GET("/health", health.Get)
}
