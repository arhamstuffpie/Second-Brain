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

type stubModelProfileRepository struct{}

func (stubModelProfileRepository) Get(context.Context, string, string) (StoredModelProfile, bool, error) {
	return StoredModelProfile{}, false, nil
}
func (stubModelProfileRepository) Upsert(_ context.Context, profile StoredModelProfile) (StoredModelProfile, error) {
	return profile, nil
}
func (stubModelProfileRepository) Delete(context.Context, string, string) error { return nil }

type stubCredentialCipher struct{}

func (stubCredentialCipher) Seal(value string) (string, error) { return "sealed:" + value, nil }
func (stubCredentialCipher) Open(value string) (string, error) {
	return strings.TrimPrefix(value, "sealed:"), nil
}

type stubSpeakerProfileRepository struct{}

func (stubSpeakerProfileRepository) FindSpeakerObservation(context.Context, string, string, string, string) (SpeakerObservation, bool, error) {
	return SpeakerObservation{}, false, nil
}
func (stubSpeakerProfileRepository) ResolveSpeakerProfile(context.Context, ResolveSpeakerProfileInput) (SpeakerProfileResolution, error) {
	return SpeakerProfileResolution{}, nil
}
func (stubSpeakerProfileRepository) CreateSpeakerSample(context.Context, CreateSpeakerSampleInput) (SpeakerSample, error) {
	return SpeakerSample{}, nil
}
func (stubSpeakerProfileRepository) CreateSpeakerObservation(context.Context, CreateSpeakerObservationInput) (SpeakerObservation, error) {
	return SpeakerObservation{}, nil
}
func (stubSpeakerProfileRepository) ListSpeakerProfiles(context.Context, string) ([]SpeakerProfile, error) {
	return []SpeakerProfile{}, nil
}
func (stubSpeakerProfileRepository) UpdateSpeakerProfile(_ context.Context, input UpdateSpeakerProfileInput) (SpeakerProfile, error) {
	return SpeakerProfile{ID: input.ID}, nil
}
func (stubSpeakerProfileRepository) DeleteSpeakerProfile(context.Context, string, string) ([]string, error) {
	return []string{}, nil
}
func (stubSpeakerProfileRepository) PurgeExpiredSpeakerProfiles(context.Context, string) ([]string, error) {
	return []string{}, nil
}
func (stubSpeakerProfileRepository) ListSpeakerSamples(context.Context, string, string) ([]SpeakerSample, error) {
	return []SpeakerSample{}, nil
}
func (stubSpeakerProfileRepository) GetSpeakerSample(context.Context, string, string, string) (SpeakerSample, error) {
	return SpeakerSample{}, nil
}

type stubPersonRepository struct{}

func (stubPersonRepository) EnrollFace(context.Context, EnrollFaceProfileInput) (PersonProfile, error) {
	return PersonProfile{}, nil
}
func (stubPersonRepository) MatchFace(context.Context, MatchFaceProfileInput) (FaceMatch, error) {
	return FaceMatch{}, nil
}
func (stubPersonRepository) ListPeople(context.Context, string) ([]PersonProfile, error) {
	return []PersonProfile{}, nil
}
func (stubPersonRepository) UpdatePerson(context.Context, UpdatePersonInput) (PersonProfile, error) {
	return PersonProfile{}, nil
}
func (stubPersonRepository) ConfirmIdentity(context.Context, ConfirmPersonIdentityInput) (PersonProfile, error) {
	return PersonProfile{}, nil
}
func (stubPersonRepository) DeletePerson(context.Context, string, string) ([]string, error) {
	return []string{}, nil
}

func (stubUserRepository) FindByEmail(context.Context, string) (StoredUser, bool, error) {
	return StoredUser{}, true, nil
}

type stubVoiceRepository struct{}

func (stubVoiceRepository) CreateEnrollmentSample(context.Context, CreateEnrollmentSampleInput) (VoiceEnrollmentRecord, error) {
	return VoiceEnrollmentRecord{}, nil
}
func (stubVoiceRepository) ListEnrollmentSamples(context.Context, string) ([]VoiceEnrollmentRecord, error) {
	return nil, nil
}
func (stubVoiceRepository) GetEnrollmentSample(context.Context, string, string) (VoiceEnrollmentRecord, error) {
	return VoiceEnrollmentRecord{}, nil
}
func (stubVoiceRepository) DeleteEnrollmentSample(context.Context, string, string) (VoiceEnrollmentRecord, error) {
	return VoiceEnrollmentRecord{}, nil
}

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
func (stubVoiceRepository) SaveTranscriptAndQueueAssembly(context.Context, VoiceJob, Transcript, []string, string, string, int) error {
	return nil
}
func (stubVoiceRepository) LoadAssembly(context.Context, VoiceJob) (VoiceAssemblySnapshot, error) {
	return VoiceAssemblySnapshot{}, nil
}
func (stubVoiceRepository) SaveAssembledEpisodes(context.Context, VoiceJob, VoiceAssemblySnapshot, []EpisodeDraft, int) error {
	return nil
}
func (stubVoiceRepository) CompleteMemographEpisode(context.Context, VoiceJob, json.RawMessage) error {
	return nil
}
func (stubVoiceRepository) RetryJob(context.Context, VoiceJob, string, time.Time, bool) error {
	return nil
}

type stubVideoRepository struct{}

func (stubVideoRepository) CreateVideoRecording(context.Context, CreateVideoRecordingInput, int) (VideoRecording, error) {
	return VideoRecording{}, nil
}
func (stubVideoRepository) CreateRealtimeVideoChunk(context.Context, CreateRealtimeVideoChunkInput, int) (VideoRecording, error) {
	return VideoRecording{}, nil
}
func (stubVideoRepository) FindVideoRecordingByClientChunk(context.Context, string, string, string) (VideoRecording, bool, error) {
	return VideoRecording{}, false, nil
}
func (stubVideoRepository) GetVideoRecording(context.Context, string, string) (VideoRecordingDetail, error) {
	return VideoRecordingDetail{}, nil
}
func (stubVideoRepository) QueueVideoReprocessing(context.Context, string, string) (VideoRecording, error) {
	return VideoRecording{}, nil
}
func (stubVideoRepository) GetVideoSourceObject(context.Context, string, string) (string, StoredObject, error) {
	return "", StoredObject{}, nil
}
func (stubVideoRepository) CreateVideoRealtimeSession(context.Context, StartVideoRealtimeSessionInput) (RealtimeVideoSession, error) {
	return RealtimeVideoSession{}, nil
}
func (stubVideoRepository) GetVideoRealtimeSession(context.Context, string, string) (RealtimeVideoSessionDetail, error) {
	return RealtimeVideoSessionDetail{}, nil
}
func (stubVideoRepository) StopVideoRealtimeSession(context.Context, string, string) (RealtimeVideoSession, error) {
	return RealtimeVideoSession{}, nil
}
func (stubVideoRepository) ClaimVideoJob(context.Context) (VideoJob, bool, error) {
	return VideoJob{}, false, nil
}
func (stubVideoRepository) CreateVideoAnalysisBatches(context.Context, VideoJob, float64, []VideoFrame) error {
	return nil
}
func (stubVideoRepository) ClaimVideoAnalysisBatch(context.Context, VideoJob) (VideoAnalysisBatch, bool, error) {
	return VideoAnalysisBatch{}, false, nil
}
func (stubVideoRepository) CompleteVideoAnalysisBatch(context.Context, VideoJob, VideoAnalysisBatch, VisualAnalysis) error {
	return nil
}
func (stubVideoRepository) RetryVideoAnalysisBatch(context.Context, VideoJob, VideoAnalysisBatch, string, bool) error {
	return nil
}
func (stubVideoRepository) FinishVideoAnalysis(context.Context, VideoJob, float64, string, string, int) (bool, error) {
	return false, nil
}
func (stubVideoRepository) SaveVideoTranscript(context.Context, VideoJob, Transcript, []string, string, string, int) error {
	return nil
}
func (stubVideoRepository) SaveVideoAnalysis(context.Context, VideoJob, float64, VisualAnalysis, string, string, int) error {
	return nil
}
func (stubVideoRepository) SaveVideoEpisodes(context.Context, VideoJob, []VideoEpisodeDraft, int) error {
	return nil
}
func (stubVideoRepository) CompleteVideoMemographBranch(context.Context, VideoJob, json.RawMessage) error {
	return nil
}
func (stubVideoRepository) RetryVideoJob(context.Context, VideoJob, string, time.Time, bool) error {
	return nil
}

type stubTranscriber struct{}

func (stubTranscriber) Transcribe(context.Context, TranscriptionInput) (Transcript, error) {
	return Transcript{}, nil
}
func (stubTranscriber) Provider() string { return "stub" }
func (stubTranscriber) Model() string    { return "stub" }

type stubSpeakerAttributor struct{}

func (stubSpeakerAttributor) Attribute(_ context.Context, input SpeakerAttributionInput) (Transcript, error) {
	return input.Transcript, nil
}

type stubAudioInspector struct{}

func (stubAudioInspector) Duration(context.Context, string) (float64, error) { return 5, nil }

type stubAudioStore struct{}

func (stubAudioStore) Save(context.Context, string, io.Reader) (StoredAudio, error) {
	return StoredAudio{}, nil
}
func (stubAudioStore) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (stubAudioStore) Delete(context.Context, string) error { return nil }

type stubMediaExtractor struct{}

func (stubMediaExtractor) ExtractAudio(context.Context, string) (ExtractedAudio, error) {
	return ExtractedAudio{Audio: io.NopCloser(strings.NewReader(""))}, nil
}
func (stubMediaExtractor) ExtractFrames(context.Context, string, time.Duration, int) (FrameExtraction, error) {
	return FrameExtraction{DurationSeconds: 1, Frames: []VideoFrame{{Image: []byte("frame")}}}, nil
}
func (stubMediaExtractor) ExtractFramesAt(_ context.Context, _ string, frames []VideoFrame) ([]VideoFrame, error) {
	return frames, nil
}

type stubVisualAnalyzer struct{}

func (stubVisualAnalyzer) Analyze(context.Context, VisualAnalysisInput) (VisualAnalysis, error) {
	return VisualAnalysis{}, nil
}
func (stubVisualAnalyzer) Provider() string { return "stub" }
func (stubVisualAnalyzer) Model() string    { return "stub" }

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
func (stubMemographClient) AnswerStream(context.Context, string, MemoryAnswerRequest) (MemoryAnswerStream, error) {
	return MemoryAnswerStream{}, nil
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
		HealthRepository:  stubHealthRepository{},
		UserRepository:    stubUserRepository{},
		ModelProfiles:     stubModelProfileRepository{},
		CredentialCipher:  stubCredentialCipher{},
		VoiceRepository:   stubVoiceRepository{},
		SpeakerProfiles:   stubSpeakerProfileRepository{},
		PersonRepository:  stubPersonRepository{},
		FaceStore:         stubAudioStore{},
		VideoRepository:   stubVideoRepository{},
		Transcriber:       stubTranscriber{},
		SpeakerAttributor: stubSpeakerAttributor{},
		AudioStore:        stubAudioStore{},
		EnrollmentStore:   stubAudioStore{},
		AudioInspector:    stubAudioInspector{},
		VideoStore:        stubAudioStore{},
		MediaExtractor:    stubMediaExtractor{},
		VisualAnalyzer:    stubVisualAnalyzer{},
		Memograph:         stubMemographClient{},
		VoiceConfig:       config.VoiceConfig{EpisodeDuration: 30 * time.Second},
		VideoConfig: config.VideoConfig{
			EpisodeDuration: 30 * time.Second, FrameInterval: 5 * time.Second, MaxFrames: 6,
		},
		WorkerConfig: config.WorkerConfig{MaxAttempts: 3},
		JWT:          testJWTConfig(),
	})
	if err != nil {
		t.Fatalf("NewContainer() error = %v", err)
	}
	if container == nil || container.Health == nil || container.Auth == nil ||
		container.Models == nil || container.Voice == nil || container.Video == nil || container.People == nil {
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
