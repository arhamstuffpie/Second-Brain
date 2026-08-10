package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

type fakeModelProfileService struct{}

func (fakeModelProfileService) GetTranscription(context.Context, string) (service.ModelProfile, error) {
	return service.ModelProfile{}, nil
}
func (fakeModelProfileService) SaveTranscription(context.Context, string, service.ModelProfileInput) (service.ModelProfile, error) {
	return service.ModelProfile{}, nil
}
func (fakeModelProfileService) ResetTranscription(context.Context, string) (service.ModelProfile, error) {
	return service.ModelProfile{}, nil
}

type fakeVoiceService struct{}

func (fakeVoiceService) EnrollVoice(context.Context, service.VoiceEnrollmentInput) (service.VoiceEnrollmentSample, error) {
	return service.VoiceEnrollmentSample{}, nil
}
func (fakeVoiceService) ListVoiceEnrollments(context.Context, string) ([]service.VoiceEnrollmentSample, error) {
	return nil, nil
}
func (fakeVoiceService) DeleteVoiceEnrollment(context.Context, string, string) error { return nil }
func (fakeVoiceService) ListSpeakerProfiles(context.Context, string) ([]service.SpeakerProfile, error) {
	return []service.SpeakerProfile{}, nil
}
func (fakeVoiceService) UpdateSpeakerProfile(context.Context, service.UpdateSpeakerProfileInput) (service.SpeakerProfile, error) {
	return service.SpeakerProfile{}, nil
}
func (fakeVoiceService) DeleteSpeakerProfile(context.Context, string, string) error { return nil }
func (fakeVoiceService) OpenSpeakerSample(context.Context, string, string, string) (service.SpeakerSampleAudio, error) {
	return service.SpeakerSampleAudio{}, nil
}

func (fakeVoiceService) Ingest(context.Context, service.VoiceIngestInput) (service.VoiceRecording, error) {
	return service.VoiceRecording{}, nil
}
func (fakeVoiceService) GetRecording(context.Context, string, string) (service.VoiceRecordingDetail, error) {
	return service.VoiceRecordingDetail{}, nil
}
func (fakeVoiceService) StartRealtimeSession(context.Context, service.StartRealtimeSessionInput) (service.RealtimeVoiceSession, error) {
	return service.RealtimeVoiceSession{}, nil
}
func (fakeVoiceService) IngestRealtimeChunk(context.Context, service.RealtimeChunkInput) (service.VoiceRecording, error) {
	return service.VoiceRecording{}, nil
}
func (fakeVoiceService) GetRealtimeSession(context.Context, string, string) (service.RealtimeVoiceSessionDetail, error) {
	return service.RealtimeVoiceSessionDetail{}, nil
}
func (fakeVoiceService) StopRealtimeSession(context.Context, string, string) (service.RealtimeVoiceSession, error) {
	return service.RealtimeVoiceSession{}, nil
}
func (fakeVoiceService) CreateMemory(context.Context, string, service.MemoryCreateRequest) (json.RawMessage, error) {
	return nil, nil
}
func (fakeVoiceService) Search(context.Context, string, service.MemorySearchRequest) (json.RawMessage, error) {
	return nil, nil
}
func (fakeVoiceService) Answer(context.Context, string, service.MemoryAnswerRequest) (json.RawMessage, error) {
	return nil, nil
}
func (fakeVoiceService) AnswerStream(context.Context, string, service.MemoryAnswerRequest) (service.MemoryAnswerStream, error) {
	return service.MemoryAnswerStream{
		Body: io.NopCloser(strings.NewReader(
			"event: token\ndata: {\"content\":\"hello\"}\n\nevent: done\ndata: [DONE]\n\n",
		)),
		ContentType: "text/event-stream",
	}, nil
}
func (fakeVoiceService) GetGraph(context.Context, string, string) (json.RawMessage, error) {
	return nil, nil
}
func (fakeVoiceService) ProcessNextJob(context.Context) (bool, error) { return false, nil }

type fakeVideoService struct{}

func (fakeVideoService) IngestVideo(context.Context, service.VideoIngestInput) (service.VideoRecording, error) {
	return service.VideoRecording{}, nil
}
func (fakeVideoService) GetVideoRecording(context.Context, string, string) (service.VideoRecordingDetail, error) {
	return service.VideoRecordingDetail{}, nil
}
func (fakeVideoService) StartVideoRealtimeSession(context.Context, service.StartVideoRealtimeSessionInput) (service.RealtimeVideoSession, error) {
	return service.RealtimeVideoSession{}, nil
}
func (fakeVideoService) IngestVideoRealtimeChunk(context.Context, service.RealtimeVideoChunkInput) (service.VideoRecording, error) {
	return service.VideoRecording{}, nil
}
func (fakeVideoService) GetVideoRealtimeSession(context.Context, string, string) (service.RealtimeVideoSessionDetail, error) {
	return service.RealtimeVideoSessionDetail{}, nil
}
func (fakeVideoService) StopVideoRealtimeSession(context.Context, string, string) (service.RealtimeVideoSession, error) {
	return service.RealtimeVideoSession{}, nil
}
func (fakeVideoService) ProcessNextVideoJob(context.Context) (bool, error) { return false, nil }

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

func TestVoiceEnrollmentRoutesRequireJWT(t *testing.T) {
	engine := testRouterWithAuth(t, fakeHealthService{}, fakeAuthService{})
	for _, target := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/v1/voice/enrollments/samples"},
		{method: http.MethodGet, path: "/api/v1/voice/enrollments/samples"},
		{method: http.MethodDelete, path: "/api/v1/voice/enrollments/samples/sample-1"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(target.method, target.path, nil)
		engine.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want 401", target.method, target.path, recorder.Code)
		}
	}
}

func TestModelProfileRoutesRequireJWT(t *testing.T) {
	engine := testRouterWithAuth(t, fakeHealthService{}, fakeAuthService{})
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, "/api/v1/model-profiles/transcription", nil)
		engine.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s model profile status = %d, want 401", method, recorder.Code)
		}
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

func TestMemoryAnswerRouteStreamsSSE(t *testing.T) {
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
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/memory/55d875c1-7e6f-43c9-a858-e084278e62a5/answer",
		bytes.NewBufferString(`{"query":"What happened?","stream":true}`),
	)
	request.Header.Set("Authorization", "Bearer "+signedToken)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q", got)
	}
	if !strings.Contains(recorder.Body.String(), `"content":"hello"`) ||
		!strings.Contains(recorder.Body.String(), "event: done") {
		t.Fatalf("body = %q, want streamed token and done events", recorder.Body.String())
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
		ModelService:  fakeModelProfileService{},
		VoiceService:  fakeVoiceService{},
		VideoService:  fakeVideoService{},
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
