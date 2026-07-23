package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/arham/ai-second-brain/internal/http/middleware"
	"github.com/arham/ai-second-brain/internal/http/response"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog"
)

type fakeHealthService struct {
	health service.Health
	err    error
}

func (f fakeHealthService) Check(context.Context) (service.Health, error) {
	return f.health, f.err
}

type fakeAuthService struct {
	signupResult service.AuthResult
	signupErr    error
	loginResult  service.AuthResult
	loginErr     error
}

func (f fakeAuthService) Signup(context.Context, string, string) (service.AuthResult, error) {
	return f.signupResult, f.signupErr
}

func (f fakeAuthService) Login(context.Context, string, string) (service.AuthResult, error) {
	return f.loginResult, f.loginErr
}

func TestHealthRoute(t *testing.T) {
	checkedAt := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	engine := testRouter(t, fakeHealthService{health: service.Health{
		Status: "ok", Database: "up", CheckedAt: checkedAt,
	}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if recorder.Header().Get(middleware.RequestIDHeader) == "" {
		t.Fatal("response has no request ID")
	}
	var envelope response.Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != "" || envelope.Message != "service is healthy" || envelope.Data == nil {
		t.Fatalf("response envelope = %+v", envelope)
	}
}

func TestSignupRoute(t *testing.T) {
	engine := testRouterWithAuth(t, fakeHealthService{}, fakeAuthService{signupResult: service.AuthResult{
		User:        service.User{ID: "user-123", Email: "user@example.com"},
		AccessToken: "signed-token",
		TokenType:   "Bearer",
	}})
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/signup",
		bytes.NewBufferString(`{"email":"user@example.com","password":"password123"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
}

func TestSecureRouteRequiresJWT(t *testing.T) {
	engine := testRouterWithAuth(t, fakeHealthService{}, fakeAuthService{})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/secure", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
}

func TestSecureRouteAcceptsJWT(t *testing.T) {
	engine := testRouterWithAuth(t, fakeHealthService{}, fakeAuthService{})
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "user-123",
		Issuer:    "test-issuer",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	signedToken, err := token.SignedString([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/secure", nil)
	request.Header.Set("Authorization", "Bearer "+signedToken)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"user_id":"user-123"`)) {
		t.Fatalf("body = %s, want authenticated user ID", recorder.Body.String())
	}
}

func TestHealthRouteWhenDatabaseIsUnavailable(t *testing.T) {
	engine := testRouter(t, fakeHealthService{err: &service.UnavailableError{Cause: errors.New("down")}})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
	var envelope response.Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != "SERVICE_UNAVAILABLE" {
		t.Fatalf("code = %q, want SERVICE_UNAVAILABLE", envelope.Code)
	}
}

func testRouter(t *testing.T, healthService service.HealthService) http.Handler {
	return testRouterWithAuth(t, healthService, fakeAuthService{})
}

func testRouterWithAuth(t *testing.T, healthService service.HealthService, authService service.AuthService) http.Handler {
	t.Helper()
	handlers, err := handler.NewContainer(handler.Dependencies{
		HealthService: healthService,
		AuthService:   authService,
	})
	if err != nil {
		t.Fatalf("handler.NewContainer() error = %v", err)
	}
	cfg := config.Config{
		Environment: "test",
		JWT: config.JWTConfig{
			Secret: "01234567890123456789012345678901", Issuer: "test-issuer",
		},
		CORS: config.CORSConfig{
			AllowedOrigins: []string{"http://localhost:3000"},
			AllowedMethods: []string{"GET", "OPTIONS"},
			AllowedHeaders: []string{"Authorization", "Content-Type", "X-Request-ID"},
			MaxAge:         600,
		},
	}
	middlewares, err := middleware.NewContainer(cfg)
	if err != nil {
		t.Fatalf("middleware.NewContainer() error = %v", err)
	}
	engine, err := New(Dependencies{
		Config: cfg, Logger: zerolog.Nop(), Handlers: handlers, Middleware: middlewares,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return engine
}
