package stt

import (
	"context"
	"strings"
	"testing"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

type profileRepositoryStub struct {
	profile service.StoredModelProfile
	found   bool
}

func (r profileRepositoryStub) Get(context.Context, string, string) (service.StoredModelProfile, bool, error) {
	return r.profile, r.found, nil
}
func (profileRepositoryStub) Upsert(_ context.Context, profile service.StoredModelProfile) (service.StoredModelProfile, error) {
	return profile, nil
}
func (profileRepositoryStub) Delete(context.Context, string, string) error { return nil }

type plaintextCipher struct{}

func (plaintextCipher) Seal(value string) (string, error) { return value, nil }
func (plaintextCipher) Open(value string) (string, error) { return value, nil }

type configuredTranscriber struct{}

func (configuredTranscriber) Provider() string { return "compatible" }
func (configuredTranscriber) Model() string    { return "owner-diarize" }
func (configuredTranscriber) Transcribe(
	_ context.Context,
	_ service.TranscriptionInput,
) (service.Transcript, error) {
	return service.Transcript{
		Text: "hello",
		Segments: []service.TranscriptSegment{{
			StartTime: 0, EndTime: 1, Speaker: "A", Text: "hello",
		}},
	}, nil
}

func TestProfileAwareTranscriberUsesOwnerProfile(t *testing.T) {
	transcriber, err := NewProfileAware(
		NewMock(),
		profileRepositoryStub{found: true, profile: service.StoredModelProfile{
			OwnerUserID: "owner-1", Task: "transcription", Provider: "compatible",
			BaseURL: "https://speech.example/v1", Model: "owner-diarize", APIKeyCiphertext: "owner-key",
		}},
		plaintextCipher{}, config.STTConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	transcriber.compatible = func(provider string, cfg config.STTConfig) (service.Transcriber, error) {
		if provider != "compatible" || cfg.BaseURL != "https://speech.example/v1" ||
			cfg.Model != "owner-diarize" || cfg.APIKey != "owner-key" {
			t.Fatalf("resolved provider = %q, config = %+v", provider, cfg)
		}
		return configuredTranscriber{}, nil
	}
	result, err := transcriber.Transcribe(context.Background(), service.TranscriptionInput{
		OwnerUserID: "owner-1", FileName: "audio.wav", MediaType: "audio/wav",
		Audio: strings.NewReader("audio"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "compatible" || result.Model != "owner-diarize" || len(result.Segments) != 1 {
		t.Fatalf("Transcribe() = %+v", result)
	}
}

func TestProfileAwareTranscriberFallsBackToServerModel(t *testing.T) {
	transcriber, err := NewProfileAware(
		NewMock(), profileRepositoryStub{}, plaintextCipher{}, config.STTConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := transcriber.Transcribe(context.Background(), service.TranscriptionInput{
		OwnerUserID: "owner-1", Audio: strings.NewReader("fallback"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "mock" || result.Model != "mock-text-audio" || result.Text != "fallback" {
		t.Fatalf("Transcribe() = %+v", result)
	}
}
