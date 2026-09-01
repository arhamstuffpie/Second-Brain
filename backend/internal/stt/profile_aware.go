package stt

import (
	"context"
	"fmt"
	"strings"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

// ProfileAwareTranscriber resolves the authenticated owner's saved model at
// job execution time. This keeps the durable worker compatible with app-level
// model changes without putting API keys into job payloads or logs.
type ProfileAwareTranscriber struct {
	fallback   service.Transcriber
	repository service.ModelProfileRepository
	cipher     service.CredentialCipher
	defaults   config.STTConfig
	compatible func(provider string, cfg config.STTConfig) (service.Transcriber, error)
}

func NewProfileAware(
	fallback service.Transcriber,
	repository service.ModelProfileRepository,
	cipher service.CredentialCipher,
	defaults config.STTConfig,
) (*ProfileAwareTranscriber, error) {
	if fallback == nil || repository == nil || cipher == nil {
		return nil, fmt.Errorf("profile-aware transcriber dependencies are required")
	}
	return &ProfileAwareTranscriber{
		fallback: fallback, repository: repository, cipher: cipher, defaults: defaults,
		compatible: func(provider string, cfg config.STTConfig) (service.Transcriber, error) {
			return NewCompatible(provider, cfg)
		},
	}, nil
}

func (t *ProfileAwareTranscriber) Provider() string { return t.fallback.Provider() }
func (t *ProfileAwareTranscriber) Model() string    { return t.fallback.Model() }

func (t *ProfileAwareTranscriber) Transcribe(
	ctx context.Context,
	input service.TranscriptionInput,
) (service.Transcript, error) {
	profile, found, err := t.repository.Get(ctx, input.OwnerUserID, "transcription")
	if err != nil {
		return service.Transcript{}, fmt.Errorf("resolve transcription model: %w", err)
	}
	transcriber := t.fallback
	if found {
		switch profile.Provider {
		case "mock":
			transcriber = NewMock()
		default:
			apiKey, decryptErr := t.cipher.Open(profile.APIKeyCiphertext)
			if decryptErr != nil {
				return service.Transcript{}, fmt.Errorf("decrypt transcription API key: %w", decryptErr)
			}
			transcriber, err = t.compatible(profile.Provider, config.STTConfig{
				BaseURL: profile.BaseURL, APIKey: apiKey, Model: profile.Model,
				Language: t.defaults.Language, Prompt: t.defaults.Prompt, Timeout: t.defaults.Timeout,
			})
			if err != nil {
				return service.Transcript{}, fmt.Errorf("configure %s transcriber: %w", profile.Provider, err)
			}
		}
	}
	transcript, err := transcriber.Transcribe(ctx, input)
	if err != nil {
		return service.Transcript{}, err
	}
	if strings.TrimSpace(transcript.Provider) == "" {
		transcript.Provider = transcriber.Provider()
	}
	if strings.TrimSpace(transcript.Model) == "" {
		transcript.Model = transcriber.Model()
	}
	return transcript, nil
}

var _ service.Transcriber = (*ProfileAwareTranscriber)(nil)
