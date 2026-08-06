package handler

import (
	"net/http"

	"github.com/arham/ai-second-brain/internal/http/response"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/arham/ai-second-brain/internal/utils"
	"github.com/gin-gonic/gin"
)

type ModelProfileHandler interface {
	GetTranscription(c *gin.Context)
	SaveTranscription(c *gin.Context)
	ResetTranscription(c *gin.Context)
}

type modelProfileHandler struct {
	service service.ModelProfileService
}

func newModelProfileHandler(modelService service.ModelProfileService) *modelProfileHandler {
	return &modelProfileHandler{service: modelService}
}

func (h *modelProfileHandler) GetTranscription(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	profile, err := h.service.GetTranscription(c.Request.Context(), principal.Subject)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, profile, "transcription model profile")
}

func (h *modelProfileHandler) SaveTranscription(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	var request service.ModelProfileInput
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid model profile request")
		return
	}
	profile, err := h.service.SaveTranscription(c.Request.Context(), principal.Subject, request)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, profile, "transcription model profile saved")
}

func (h *modelProfileHandler) ResetTranscription(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	profile, err := h.service.ResetTranscription(c.Request.Context(), principal.Subject)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, profile, "server transcription model restored")
}

var _ ModelProfileHandler = (*modelProfileHandler)(nil)
