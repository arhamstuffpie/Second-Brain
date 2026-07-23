package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestJWTAuthenticator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "01234567890123456789012345678901"
	authenticator, err := NewJWTAuthenticator(config.JWTConfig{Secret: secret, Issuer: "test-issuer"})
	if err != nil {
		t.Fatalf("NewJWTAuthenticator() error = %v", err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "user-123",
		Issuer:    "test-issuer",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	engine := gin.New()
	engine.GET("/protected", authenticator.Handle(), func(c *gin.Context) {
		principal, ok := utils.PrincipalFromContext(c.Request.Context())
		if !ok || principal.Subject != "user-123" {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+signed)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
}

func TestJWTAuthenticatorRejectsMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator, err := NewJWTAuthenticator(config.JWTConfig{
		Secret: "01234567890123456789012345678901", Issuer: "test-issuer",
	})
	if err != nil {
		t.Fatalf("NewJWTAuthenticator() error = %v", err)
	}

	engine := gin.New()
	engine.GET("/protected", authenticator.Handle(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}
