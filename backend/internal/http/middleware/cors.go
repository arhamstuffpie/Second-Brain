package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/http/response"
	"github.com/gin-gonic/gin"
)

func CORS(cfg config.CORSConfig) gin.HandlerFunc {
	allowedOrigins := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		allowedOrigins[strings.TrimSpace(origin)] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		_, explicitlyAllowed := allowedOrigins[origin]
		_, allowAny := allowedOrigins["*"]
		if origin != "" && (explicitlyAllowed || allowAny) {
			allowedOrigin := origin
			if allowAny {
				allowedOrigin = "*"
			}
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
			c.Header("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ", "))
			c.Header("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))
			c.Header("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
			c.Header("Vary", "Origin")
		}

		if c.Request.Method == http.MethodOptions {
			if origin != "" && !explicitlyAllowed && !allowAny {
				response.Error(c, http.StatusForbidden, "CORS_ORIGIN_FORBIDDEN", "origin is not allowed")
				c.Abort()
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
