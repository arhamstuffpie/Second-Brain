package handler

import (
	"net/http"

	"github.com/arham/ai-second-brain/internal/http/response"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/arham/ai-second-brain/internal/utils"
	"github.com/gin-gonic/gin"
)

type AuthHandler interface {
	Signup(c *gin.Context)
	Login(c *gin.Context)
	Secure(c *gin.Context)
}

type authHandler struct {
	service service.AuthService
}

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type secureResponse struct {
	UserID string `json:"user_id"`
}

func newAuthHandler(service service.AuthService) *authHandler {
	return &authHandler{service: service}
}

func (h *authHandler) Signup(c *gin.Context) {
	var request credentialsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "request body must contain email and password")
		return
	}

	result, err := h.service.Signup(c.Request.Context(), request.Email, request.Password)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, result, "account created")
}

func (h *authHandler) Login(c *gin.Context) {
	var request credentialsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "request body must contain email and password")
		return
	}

	result, err := h.service.Login(c.Request.Context(), request.Email, request.Password)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "login successful")
}

func (h *authHandler) Secure(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	response.Success(c, http.StatusOK, secureResponse{UserID: principal.Subject}, "authenticated request successful")
}

var _ AuthHandler = (*authHandler)(nil)
