package response

import (
	"errors"
	"net/http"

	"github.com/arham/ai-second-brain/internal/service"
	"github.com/gin-gonic/gin"
)

type Envelope struct {
	Data    any     `json:"data"`
	Error   string  `json:"error"`
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Paging  *Paging `json:"paging"`
}

type Paging struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

func Success(c *gin.Context, status int, data any, message string) {
	c.JSON(status, Envelope{Data: data, Message: message})
}

func Error(c *gin.Context, status int, code, message string) {
	c.JSON(status, Envelope{Error: http.StatusText(status), Code: code, Message: message})
}

func ServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrValidation):
		Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, service.ErrUnauthorized):
		Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
	case errors.Is(err, service.ErrForbidden):
		Error(c, http.StatusForbidden, "FORBIDDEN", "access is forbidden")
	case errors.Is(err, service.ErrNotFound):
		Error(c, http.StatusNotFound, "NOT_FOUND", "resource was not found")
	case errors.Is(err, service.ErrConflict):
		Error(c, http.StatusConflict, "CONFLICT", "resource conflicts with existing state")
	default:
		var unavailable *service.UnavailableError
		if errors.As(err, &unavailable) {
			Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "a required dependency is unavailable")
			return
		}
		Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
	}
}
