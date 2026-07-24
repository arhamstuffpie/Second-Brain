package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/arham/ai-second-brain/internal/service"
)

type stubHealthService struct{}

func (stubHealthService) Check(context.Context) (service.Health, error) {
	return service.Health{}, nil
}

type stubAuthService struct{}

func (stubAuthService) Signup(context.Context, string, string) (service.AuthResult, error) {
	return service.AuthResult{}, nil
}

func (stubAuthService) Login(context.Context, string, string) (service.AuthResult, error) {
	return service.AuthResult{}, nil
}

type stubVoiceService struct{}

func (stubVoiceService) Ingest(context.Context, service.VoiceIngestInput) (service.VoiceRecording, error) {
	return service.VoiceRecording{}, nil
}
func (stubVoiceService) GetRecording(context.Context, string, string) (service.VoiceRecordingDetail, error) {
	return service.VoiceRecordingDetail{}, nil
}
func (stubVoiceService) StartRealtimeSession(context.Context, service.StartRealtimeSessionInput) (service.RealtimeVoiceSession, error) {
	return service.RealtimeVoiceSession{}, nil
}
func (stubVoiceService) IngestRealtimeChunk(context.Context, service.RealtimeChunkInput) (service.VoiceRecording, error) {
	return service.VoiceRecording{}, nil
}
func (stubVoiceService) GetRealtimeSession(context.Context, string, string) (service.RealtimeVoiceSessionDetail, error) {
	return service.RealtimeVoiceSessionDetail{}, nil
}
func (stubVoiceService) StopRealtimeSession(context.Context, string, string) (service.RealtimeVoiceSession, error) {
	return service.RealtimeVoiceSession{}, nil
}
func (stubVoiceService) CreateMemory(context.Context, string, service.MemoryCreateRequest) (json.RawMessage, error) {
	return nil, nil
}
func (stubVoiceService) Search(context.Context, string, service.MemorySearchRequest) (json.RawMessage, error) {
	return nil, nil
}
func (stubVoiceService) Answer(context.Context, string, service.MemoryAnswerRequest) (json.RawMessage, error) {
	return nil, nil
}
func (stubVoiceService) GetGraph(context.Context, string, string) (json.RawMessage, error) {
	return nil, nil
}
func (stubVoiceService) ProcessNextJob(context.Context) (bool, error) { return false, nil }

func TestNewContainerRequiresServices(t *testing.T) {
	if _, err := NewContainer(Dependencies{}); err == nil {
		t.Fatal("NewContainer() error = nil, want error")
	}
}

func TestNewContainerPopulatesDependencies(t *testing.T) {
	container, err := NewContainer(Dependencies{
		HealthService: stubHealthService{},
		AuthService:   stubAuthService{},
		VoiceService:  stubVoiceService{},
	})
	if err != nil {
		t.Fatalf("NewContainer() error = %v", err)
	}
	if container == nil || container.Health == nil || container.Auth == nil || container.Voice == nil {
		t.Fatal("handler container has nil required dependency")
	}
}
