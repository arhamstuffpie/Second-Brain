package service

import (
	"context"
	"strings"
	"testing"

	"github.com/arham/ai-second-brain/internal/config"
)

type memoryModelProfileRepository struct {
	profile StoredModelProfile
	found   bool
}

func (r *memoryModelProfileRepository) Get(context.Context, string, string) (StoredModelProfile, bool, error) {
	return r.profile, r.found, nil
}
func (r *memoryModelProfileRepository) Upsert(_ context.Context, profile StoredModelProfile) (StoredModelProfile, error) {
	r.profile = profile
	r.profile.ID = "profile-1"
	r.found = true
	return r.profile, nil
}
func (r *memoryModelProfileRepository) Delete(context.Context, string, string) error {
	r.profile = StoredModelProfile{}
	r.found = false
	return nil
}

func TestModelProfileServiceUsesServerDefault(t *testing.T) {
	service := newModelProfileService(
		&memoryModelProfileRepository{}, stubCredentialCipher{},
		config.STTConfig{
			Provider: "openai", BaseURL: "https://api.openai.com/v1",
			Model: "server-model", APIKey: "server-key",
		},
	)
	profile, err := service.GetTranscription(context.Background(), "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Source != "server" || profile.Model != "server-model" || !profile.APIKeyConfigured {
		t.Fatalf("GetTranscription() = %+v", profile)
	}
}

func TestModelProfileServiceSavesAndRetainsEncryptedKey(t *testing.T) {
	repository := &memoryModelProfileRepository{}
	service := newModelProfileService(repository, stubCredentialCipher{}, config.STTConfig{})
	profile, err := service.SaveTranscription(context.Background(), "owner-1", ModelProfileInput{
		Provider: "OpenAI", Model: "gpt-4o-transcribe-diarize", APIKey: "secret-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Source != "account" || profile.BaseURL != "https://api.openai.com/v1" ||
		!profile.APIKeyConfigured || repository.profile.APIKeyCiphertext != "sealed:secret-key" {
		t.Fatalf("SaveTranscription() = %+v, stored = %+v", profile, repository.profile)
	}

	_, err = service.SaveTranscription(context.Background(), "owner-1", ModelProfileInput{
		Provider: "openai", BaseURL: "https://speech.example/v1", Model: "new-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.profile.APIKeyCiphertext != "sealed:secret-key" {
		t.Fatalf("retained ciphertext = %q", repository.profile.APIKeyCiphertext)
	}

	_, err = service.SaveTranscription(context.Background(), "owner-1", ModelProfileInput{
		Provider: "compatible", BaseURL: "https://other.example/v1", Model: "other-model",
	})
	if err == nil || !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("provider change error = %v, want new API key validation", err)
	}
}

func TestModelProfileServiceValidatesCompatibleProvider(t *testing.T) {
	service := newModelProfileService(
		&memoryModelProfileRepository{}, stubCredentialCipher{}, config.STTConfig{},
	)
	_, err := service.SaveTranscription(context.Background(), "owner-1", ModelProfileInput{
		Provider: "custom provider", Model: "model", APIKey: "key",
	})
	if err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("SaveTranscription() error = %v, want provider validation", err)
	}
	_, err = service.SaveTranscription(context.Background(), "owner-1", ModelProfileInput{
		Provider: "custom", Model: "model", APIKey: "key", BaseURL: "speech.example/v1",
	})
	if err == nil || !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("SaveTranscription() error = %v, want base_url validation", err)
	}
}

func TestModelProfileServiceResetDeletesAccountCredential(t *testing.T) {
	repository := &memoryModelProfileRepository{
		found: true,
		profile: StoredModelProfile{
			OwnerUserID: "owner-1", Task: "transcription", Provider: "custom",
			Model: "model", BaseURL: "https://speech.example/v1", APIKeyCiphertext: "sealed:key",
		},
	}
	service := newModelProfileService(
		repository, stubCredentialCipher{}, config.STTConfig{Provider: "mock", Model: "mock-text-audio"},
	)
	profile, err := service.ResetTranscription(context.Background(), "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if repository.found || profile.Source != "server" || profile.Provider != "mock" {
		t.Fatalf("ResetTranscription() = %+v, repository found = %t", profile, repository.found)
	}
}
