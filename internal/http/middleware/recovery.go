package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/arham/ai-second-brain/internal/http/response"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func Recovery(logger zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error().
					Interface("panic", recovered).
					Bytes("stack", debug.Stack()).
					Str("request_id", RequestIDFromContext(c)).
					Msg("recovered from panic")
				c.Abort()
				if !c.Writer.Written() {
					response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
				}
			}
		}()
		c.Next()
	}
}
