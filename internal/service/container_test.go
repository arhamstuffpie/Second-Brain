package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

type stubHealthRepository struct {
	err error
}

func (s stubHealthRepository) Ping(context.Context) error {
	return s.err
}

type stubUserRepository struct{}

func (stubUserRepository) Create(context.Context, string, string) (StoredUser, bool, error) {
	return StoredUser{}, true, nil
}

func (stubUserRepository) FindByEmail(context.Context, string) (StoredUser, bool, error) {
	return StoredUser{}, true, nil
}

type stubVoiceRepository struct{}

func (stubVoiceRepository) CreateRecording(context.Context, CreateRecordingInput, int) (VoiceRecording, error) {
	return VoiceRecording{}, nil
}
func (stubVoiceRepository) FindRecordingByChunk(context.Context, string, string, int) (VoiceRecording, bool, error) {
	return VoiceRecording{}, false, nil
}
func (stubVoiceRepository) GetRecording(context.Context, string, string) (VoiceRecordingDetail, error) {
	return VoiceRecordingDetail{}, nil
}
func (stubVoiceRepository) CreateRealtimeSession(context.Context, StartRealtimeSessionInput) (RealtimeVoiceSession, error) {
	return RealtimeVoiceSession{}, nil
}
func (stubVoiceRepository) GetRealtimeSession(context.Context, string, string) (RealtimeVoiceSessionDetail, error) {
	return RealtimeVoiceSessionDetail{}, nil
}
func (stubVoiceRepository) StopRealtimeSession(context.Context, string, string) (RealtimeVoiceSession, error) {
	return RealtimeVoiceSession{}, nil
}
func (stubVoiceRepository) ClaimJob(context.Context) (VoiceJob, bool, error) {
	return VoiceJob{}, false, nil
}
func (stubVoiceRepository) SaveTranscriptAndEpisodes(context.Context, VoiceJob, Transcript, []EpisodeDraft, string, string, int) error {
	return nil
}
func (stubVoiceRepository) CompleteMemographEpisode(context.Context, VoiceJob, json.RawMessage) error {
	return nil
}
func (stubVoiceRepository) RetryJob(context.Context, VoiceJob, string, time.Time, bool) error {
	return nil
}

type stubTranscriber struct{}

func (stubTranscriber) Transcribe(context.Context, TranscriptionInput) (Transcript, error) {
	return Transcript{}, nil
}
func (stubTranscriber) Provider() string { return "stub" }
func (stubTranscriber) Model() string    { return "stub" }

type stubAudioStore struct{}

func (stubAudioStore) Save(context.Context, string, io.Reader) (StoredAudio, error) {
	return StoredAudio{}, nil
}
func (stubAudioStore) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (stubAudioStore) Delete(context.Context, string) error { return nil }

type stubMemographClient struct{}

func (stubMemographClient) CreateMemory(context.Context, string, MemoryCreateRequest) (json.RawMessage, error) {
	return nil, nil
}
func (stubMemographClient) InsertEpisode(context.Context, string, EpisodeInsertRequest) (json.RawMessage, error) {
	return nil, nil
}
func (stubMemographClient) Search(context.Context, string, MemorySearchRequest) (json.RawMessage, error) {
	return nil, nil
}
func (stubMemographClient) Answer(context.Context, string, MemoryAnswerRequest) (json.RawMessage, error) {
	return nil, nil
}
func (stubMemographClient) GetGraph(context.Context, string, string) (json.RawMessage, error) {
	return nil, nil
}

func testJWTConfig() config.JWTConfig {
	return config.JWTConfig{
		Secret:         "01234567890123456789012345678901",
		Issuer:         "test-issuer",
		AccessTokenTTL: time.Hour,
	}
}

func TestNewContainerRequiresRepositories(t *testing.T) {
	if _, err := NewContainer(Dependencies{}); err == nil {
		t.Fatal("NewContainer() error = nil, want error")
	}
}

func TestNewContainerPopulatesDependencies(t *testing.T) {
	container, err := NewContainer(Dependencies{
		HealthRepository: stubHealthRepository{},
		UserRepository:   stubUserRepository{},
		VoiceRepository:  stubVoiceRepository{},
		Transcriber:      stubTranscriber{},
		AudioStore:       stubAudioStore{},
		Memograph:        stubMemographClient{},
		VoiceConfig:      config.VoiceConfig{EpisodeDuration: 30 * time.Second},
		WorkerConfig:     config.WorkerConfig{MaxAttempts: 3},
		JWT:              testJWTConfig(),
	})
	if err != nil {
		t.Fatalf("NewContainer() error = %v", err)
	}
	if container == nil || container.Health == nil || container.Auth == nil || container.Voice == nil {
		t.Fatal("service container has nil required dependency")
	}
}

func TestHealthServiceWrapsDependencyFailure(t *testing.T) {
	wantErr := errors.New("database down")
	service := newHealthService(stubHealthRepository{err: wantErr})

	_, err := service.Check(context.Background())
	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("Check() error = %v, want UnavailableError", err)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("Check() error does not wrap repository error")
	}
}

func TestHealthServiceReturnsHealthyState(t *testing.T) {
	service := newHealthService(stubHealthRepository{})
	health, err := service.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if health.Status != "ok" || health.Database != "up" || health.CheckedAt.IsZero() {
		t.Fatalf("Check() = %+v, want healthy state", health)
	}
}
