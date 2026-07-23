package routes

import (
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerAuth(router gin.IRoutes, auth handler.AuthHandler) {
	router.POST("/signup", auth.Signup)
	router.POST("/login", auth.Login)
}

func registerSecure(router gin.IRoutes, auth handler.AuthHandler) {
	router.GET("/secure", auth.Secure)
}
