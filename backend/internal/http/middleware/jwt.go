package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/http/response"
	"github.com/arham/ai-second-brain/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type JWTAuthenticator interface {
	Handle() gin.HandlerFunc
}

type jwtAuthenticator struct {
	secret []byte
	issuer string
	parser *jwt.Parser
}

func NewJWTAuthenticator(cfg config.JWTConfig) (JWTAuthenticator, error) {
	if len(cfg.Secret) < 32 {
		return nil, fmt.Errorf("JWT secret must be at least 32 characters")
	}
	if strings.TrimSpace(cfg.Issuer) == "" {
		return nil, fmt.Errorf("JWT issuer is required")
	}
	return &jwtAuthenticator{
		secret: []byte(cfg.Secret),
		issuer: cfg.Issuer,
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithIssuer(cfg.Issuer),
			jwt.WithExpirationRequired(),
		),
	}, nil
}

func (a *jwtAuthenticator) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "a bearer token is required")
			c.Abort()
			return
		}

		claims := &jwt.RegisteredClaims{}
		token, err := a.parser.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
			return a.secret, nil
		})
		if err != nil || !token.Valid || claims.Subject == "" {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "the bearer token is invalid or expired")
			c.Abort()
			return
		}

		principal := utils.Principal{Subject: claims.Subject, Issuer: a.issuer}
		c.Request = c.Request.WithContext(utils.WithPrincipal(c.Request.Context(), principal))
		c.Next()
	}
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

var _ JWTAuthenticator = (*jwtAuthenticator)(nil)
