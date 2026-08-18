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

type stubModelProfileService struct{}

func (stubModelProfileService) GetTranscription(context.Context, string) (service.ModelProfile, error) {
	return service.ModelProfile{}, nil
}
func (stubModelProfileService) SaveTranscription(context.Context, string, service.ModelProfileInput) (service.ModelProfile, error) {
	return service.ModelProfile{}, nil
}
func (stubModelProfileService) ResetTranscription(context.Context, string) (service.ModelProfile, error) {
	return service.ModelProfile{}, nil
}

type stubVoiceService struct{}

func (stubVoiceService) EnrollVoice(context.Context, service.VoiceEnrollmentInput) (service.VoiceEnrollmentSample, error) {
	return service.VoiceEnrollmentSample{}, nil
}
func (stubVoiceService) ListVoiceEnrollments(context.Context, string) ([]service.VoiceEnrollmentSample, error) {
	return nil, nil
}
func (stubVoiceService) DeleteVoiceEnrollment(context.Context, string, string) error { return nil }
func (stubVoiceService) ListSpeakerProfiles(context.Context, string) ([]service.SpeakerProfile, error) {
	return []service.SpeakerProfile{}, nil
}
func (stubVoiceService) UpdateSpeakerProfile(context.Context, service.UpdateSpeakerProfileInput) (service.SpeakerProfile, error) {
	return service.SpeakerProfile{}, nil
}
func (stubVoiceService) DeleteSpeakerProfile(context.Context, string, string) error { return nil }
func (stubVoiceService) OpenSpeakerSample(context.Context, string, string, string) (service.SpeakerSampleAudio, error) {
	return service.SpeakerSampleAudio{}, nil
}

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
func (stubVoiceService) AnswerStream(context.Context, string, service.MemoryAnswerRequest) (service.MemoryAnswerStream, error) {
	return service.MemoryAnswerStream{}, nil
}
func (stubVoiceService) GetGraph(context.Context, string, string) (json.RawMessage, error) {
	return nil, nil
}
func (stubVoiceService) ProcessNextJob(context.Context) (bool, error) { return false, nil }

type stubVideoService struct{}

func (stubVideoService) ReprocessVideo(context.Context, string, string) (service.VideoRecording, error) {
	return service.VideoRecording{}, nil
}
func (stubVideoService) GetVideoEvidenceURL(context.Context, string, string, float64) (service.EvidencePlayback, error) {
	return service.EvidencePlayback{}, nil
}

func (stubVideoService) IngestVideo(context.Context, service.VideoIngestInput) (service.VideoRecording, error) {
	return service.VideoRecording{}, nil
}
func (stubVideoService) GetVideoRecording(context.Context, string, string) (service.VideoRecordingDetail, error) {
	return service.VideoRecordingDetail{}, nil
}
func (stubVideoService) StartVideoRealtimeSession(context.Context, service.StartVideoRealtimeSessionInput) (service.RealtimeVideoSession, error) {
	return service.RealtimeVideoSession{}, nil
}
func (stubVideoService) IngestVideoRealtimeChunk(context.Context, service.RealtimeVideoChunkInput) (service.VideoRecording, error) {
	return service.VideoRecording{}, nil
}
func (stubVideoService) GetVideoRealtimeSession(context.Context, string, string) (service.RealtimeVideoSessionDetail, error) {
	return service.RealtimeVideoSessionDetail{}, nil
}
func (stubVideoService) StopVideoRealtimeSession(context.Context, string, string) (service.RealtimeVideoSession, error) {
	return service.RealtimeVideoSession{}, nil
}
func (stubVideoService) ProcessNextVideoJob(context.Context) (bool, error) { return false, nil }

type stubPersonService struct{}

func (stubPersonService) EnrollFace(context.Context, service.FaceEnrollmentInput) (service.PersonProfile, error) {
	return service.PersonProfile{}, nil
}
func (stubPersonService) RecognizeFace(context.Context, service.FaceRecognitionRequest) (service.FaceMatch, error) {
	return service.FaceMatch{}, nil
}
func (stubPersonService) ListPeople(context.Context, string) ([]service.PersonProfile, error) {
	return []service.PersonProfile{}, nil
}
func (stubPersonService) UpdatePerson(context.Context, service.UpdatePersonInput) (service.PersonProfile, error) {
	return service.PersonProfile{}, nil
}
func (stubPersonService) ConfirmIdentity(context.Context, service.ConfirmPersonIdentityInput) (service.PersonProfile, error) {
	return service.PersonProfile{}, nil
}
func (stubPersonService) DeletePerson(context.Context, string, string) error { return nil }

func TestNewContainerRequiresServices(t *testing.T) {
	if _, err := NewContainer(Dependencies{}); err == nil {
		t.Fatal("NewContainer() error = nil, want error")
	}
}

func TestNewContainerPopulatesDependencies(t *testing.T) {
	container, err := NewContainer(Dependencies{
		HealthService: stubHealthService{},
		AuthService:   stubAuthService{},
		ModelService:  stubModelProfileService{},
		VoiceService:  stubVoiceService{},
		VideoService:  stubVideoService{},
		PersonService: stubPersonService{},
	})
	if err != nil {
		t.Fatalf("NewContainer() error = %v", err)
	}
	if container == nil || container.Health == nil || container.Auth == nil ||
		container.Models == nil || container.Voice == nil || container.Video == nil || container.People == nil {
		t.Fatal("handler container has nil required dependency")
	}
}
