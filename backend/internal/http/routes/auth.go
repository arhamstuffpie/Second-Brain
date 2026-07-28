package routes

import (
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerAuth(router gin.IRoutes, auth handler.AuthHandler) {
	// POST /api/v1/auth/signup
	// JSON: {"email": string, "password": string}. Creates an account and
	// returns 201 with service.AuthResult (user plus a Bearer access token).
	router.POST("/signup", auth.Signup)

	// POST /api/v1/auth/login
	// JSON: {"email": string, "password": string}. Verifies the credentials and
	// returns 200 with the same service.AuthResult shape as signup.
	router.POST("/login", auth.Login)
}

func registerSecure(router gin.IRoutes, auth handler.AuthHandler) {
	// GET /api/v1/secure
	// Requires Authorization: Bearer <access_token>. Takes no body or query
	// parameters and returns 200 with {"user_id": "<authenticated subject>"}.
	router.GET("/secure", auth.Secure)
}
