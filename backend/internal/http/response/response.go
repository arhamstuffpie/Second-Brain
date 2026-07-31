package response

import (
	"errors"
	"net/http"
	"strings"

	"github.com/arham/ai-second-brain/internal/service"
	"github.com/gin-gonic/gin"
)

// Envelope is the stable top-level contract consumed by frontend clients.
// Fields intentionally do not use omitempty: success responses include empty
// error/code values, while error responses include a null data value.
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
			if strings.EqualFold(unavailable.Dependency, "memograph") {
				Error(
					c,
					http.StatusServiceUnavailable,
					"MEMOGRAPH_UNAVAILABLE",
					readableMemographError(unavailable.Cause),
				)
				return
			}
			Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "a required dependency is unavailable")
			return
		}
		Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
	}
}

func readableMemographError(err error) string {
	if err == nil {
		return "Memograph is temporarily unavailable"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "api key or jwt is required"),
		strings.Contains(message, "not configured"):
		return "Memograph credentials are missing on the backend"
	case strings.Contains(message, "returned 401"),
		strings.Contains(message, "returned 403"):
		return "Memograph rejected the configured API key or JWT"
	case strings.Contains(message, "returned 404"):
		return "Memograph could not find the requested project or memory"
	case strings.Contains(message, "returned 429"):
		return "Memograph rate limit was reached; try again shortly"
	case strings.Contains(message, "returned 400"),
		strings.Contains(message, "returned 422"):
		return "Memograph rejected the request; check the project, memory, and group settings"
	case strings.Contains(message, "call memograph"),
		strings.Contains(message, "deadline exceeded"):
		return "The backend could not reach Memograph"
	default:
		return "Memograph is temporarily unavailable"
	}
}
