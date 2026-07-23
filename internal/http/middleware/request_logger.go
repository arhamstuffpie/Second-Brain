package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func RequestLogger(logger zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()
		latency := time.Since(startedAt).Round(time.Microsecond)

		event := logger.Info()
		if len(c.Errors) > 0 || c.Writer.Status() >= 500 {
			event = logger.Error()
		} else if c.Writer.Status() >= 400 {
			event = logger.Warn()
		}

		event.
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Str("latency", latency.String()).
			Str("request_id", RequestIDFromContext(c)).
			Str("client_ip", c.ClientIP()).
			Int("response_bytes", c.Writer.Size()).
			Int64("request_bytes", c.Request.ContentLength)
		if c.Request.URL.RawQuery != "" {
			event = event.Str("query", c.Request.URL.RawQuery)
		}
		if userAgent := c.Request.UserAgent(); userAgent != "" {
			event = event.Str("user_agent", userAgent)
		}
		if lastError := c.Errors.Last(); lastError != nil {
			event = event.Err(lastError.Err)
		}
		event.Msg("http request")
	}
}
