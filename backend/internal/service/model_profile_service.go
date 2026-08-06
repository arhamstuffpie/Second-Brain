package service

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/arham/ai-second-brain/internal/config"
)

const transcriptionTask = "transcription"

var providerNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type modelProfileService struct {
	repository ModelProfileRepository
	cipher     CredentialCipher
	fallback   config.STTConfig
}

func newModelProfileService(
	repository ModelProfileRepository,
	cipher CredentialCipher,
	fallback config.STTConfig,
) *modelProfileService {
	return &modelProfileService{repository: repository, cipher: cipher, fallback: fallback}
}

func (s *modelProfileService) GetTranscription(
	ctx context.Context,
	ownerUserID string,
) (ModelProfile, error) {
	if strings.TrimSpace(ownerUserID) == "" {
		return ModelProfile{}, ErrUnauthorized
	}
	stored, found, err := s.repository.Get(ctx, ownerUserID, transcriptionTask)
	if err != nil {
		return ModelProfile{}, err
	}
	if !found {
		return s.defaultTranscription(), nil
	}
	return publicModelProfile(stored), nil
}

func (s *modelProfileService) SaveTranscription(
	ctx context.Context,
	ownerUserID string,
	input ModelProfileInput,
) (ModelProfile, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return ModelProfile{}, ErrUnauthorized
	}
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	if !providerNamePattern.MatchString(provider) {
		return ModelProfile{}, validation(
			"provider", "must use 1–64 lowercase letters, numbers, dots, dashes, or underscores",
		)
	}
	model := strings.TrimSpace(input.Model)
	if model == "" || len(model) > 200 {
		return ModelProfile{}, validation("model", "is required and must be at most 200 characters")
	}
	baseURL, err := normalizeModelBaseURL(provider, input.BaseURL)
	if err != nil {
		return ModelProfile{}, err
	}

	current, found, err := s.repository.Get(ctx, ownerUserID, transcriptionTask)
	if err != nil {
		return ModelProfile{}, err
	}
	ciphertext := ""
	switch {
	case provider == "mock":
		ciphertext = ""
	case input.ClearAPIKey:
		ciphertext = ""
	case strings.TrimSpace(input.APIKey) != "":
		ciphertext, err = s.cipher.Seal(strings.TrimSpace(input.APIKey))
		if err != nil {
			return ModelProfile{}, fmt.Errorf("encrypt model API key: %w", err)
		}
	case found && current.Provider == provider:
		ciphertext = current.APIKeyCiphertext
	}
	if provider != "mock" && ciphertext == "" {
		return ModelProfile{}, validation("api_key", "is required for this provider")
	}

	stored, err := s.repository.Upsert(ctx, StoredModelProfile{
		OwnerUserID: ownerUserID, Task: transcriptionTask, Provider: provider,
		BaseURL: baseURL, Model: model, APIKeyCiphertext: ciphertext,
	})
	if err != nil {
		return ModelProfile{}, err
	}
	return publicModelProfile(stored), nil
}

func (s *modelProfileService) ResetTranscription(
	ctx context.Context,
	ownerUserID string,
) (ModelProfile, error) {
	if strings.TrimSpace(ownerUserID) == "" {
		return ModelProfile{}, ErrUnauthorized
	}
	if err := s.repository.Delete(ctx, ownerUserID, transcriptionTask); err != nil {
		return ModelProfile{}, err
	}
	return s.defaultTranscription(), nil
}

func (s *modelProfileService) defaultTranscription() ModelProfile {
	return ModelProfile{
		Task: transcriptionTask, Provider: s.fallback.Provider,
		BaseURL: s.fallback.BaseURL, Model: s.fallback.Model,
		APIKeyConfigured: strings.TrimSpace(s.fallback.APIKey) != "", Source: "server",
	}
}

func publicModelProfile(stored StoredModelProfile) ModelProfile {
	updatedAt := stored.UpdatedAt
	return ModelProfile{
		Task: stored.Task, Provider: stored.Provider, BaseURL: stored.BaseURL,
		Model: stored.Model, APIKeyConfigured: stored.APIKeyCiphertext != "",
		Source: "account", UpdatedAt: &updatedAt,
	}
}

func normalizeModelBaseURL(provider, raw string) (string, error) {
	if provider == "mock" {
		return "", nil
	}
	baseURL := strings.TrimRight(strings.TrimSpace(raw), "/")
	if baseURL == "" && provider == "openai" {
		baseURL = "https://api.openai.com/v1"
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", validation(
			"base_url", "must be a complete http:// or https:// API URL without credentials, query, or fragment",
		)
	}
	return baseURL, nil
}

func transcriptionProvider(transcript Transcript, fallback Transcriber) string {
	if provider := strings.TrimSpace(transcript.Provider); provider != "" {
		return provider
	}
	return fallback.Provider()
}

func transcriptionModel(transcript Transcript, fallback Transcriber) string {
	if model := strings.TrimSpace(transcript.Model); model != "" {
		return model
	}
	return fallback.Model()
}

var _ ModelProfileService = (*modelProfileService)(nil)
