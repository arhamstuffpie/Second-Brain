package handler

import (
	"net/http"

	"github.com/arham/ai-second-brain/internal/http/response"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/gin-gonic/gin"
)

type HealthHandler interface {
	Get(c *gin.Context)
}

type healthHandler struct {
	service service.HealthService
}

func newHealthHandler(service service.HealthService) *healthHandler {
	return &healthHandler{service: service}
}

func (h *healthHandler) Get(c *gin.Context) {
	health, err := h.service.Check(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, health, "service is healthy")
}

var _ HealthHandler = (*healthHandler)(nil)
