package routes

import (
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerHealth(router gin.IRoutes, health handler.HealthHandler) {
	router.GET("/health", health.Get)
}
