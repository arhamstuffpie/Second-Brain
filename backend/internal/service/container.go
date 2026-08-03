package service

import (
	"fmt"

	"github.com/arham/ai-second-brain/internal/config"
)

type Dependencies struct {
	HealthRepository  HealthRepository
	UserRepository    UserRepository
	JWT               config.JWTConfig
	VoiceRepository   VoiceRepository
	VideoRepository   VideoRepository
	Transcriber       Transcriber
	SpeakerAttributor SpeakerAttributor
	AudioStore        AudioStore
	EnrollmentStore   AudioStore
	AudioInspector    AudioInspector
	VideoStore        VideoStore
	MediaExtractor    MediaExtractor
	VisualAnalyzer    VisualAnalyzer
	Memograph         MemographClient
	VoiceConfig       config.VoiceConfig
	VideoConfig       config.VideoConfig
	WorkerConfig      config.WorkerConfig
}

type Container struct {
	Health HealthService
	Auth   AuthService
	Voice  VoiceService
	Video  VideoService
}

func NewContainer(deps Dependencies) (*Container, error) {
	if deps.HealthRepository == nil {
		return nil, fmt.Errorf("health repository is required")
	}
	if deps.UserRepository == nil {
		return nil, fmt.Errorf("user repository is required")
	}
	if deps.VoiceRepository == nil || deps.Transcriber == nil || deps.SpeakerAttributor == nil ||
		deps.AudioStore == nil || deps.EnrollmentStore == nil || deps.AudioInspector == nil ||
		deps.Memograph == nil {
		return nil, fmt.Errorf("voice service dependencies are required")
	}
	if deps.VideoRepository == nil || deps.VideoStore == nil ||
		deps.MediaExtractor == nil || deps.VisualAnalyzer == nil {
		return nil, fmt.Errorf("video service dependencies are required")
	}
	if len(deps.JWT.Secret) < 32 || deps.JWT.Issuer == "" || deps.JWT.AccessTokenTTL <= 0 {
		return nil, fmt.Errorf("valid JWT configuration is required")
	}

	container := &Container{
		Health: newHealthService(deps.HealthRepository),
		Auth:   newAuthService(deps.UserRepository, deps.JWT),
		Voice: newVoiceService(
			deps.VoiceRepository, deps.Transcriber, deps.SpeakerAttributor,
			deps.AudioStore, deps.EnrollmentStore, deps.AudioInspector, deps.Memograph,
			deps.VoiceConfig, deps.WorkerConfig,
		),
		Video: newVideoService(
			deps.VideoRepository, deps.Transcriber, deps.VideoStore,
			deps.MediaExtractor, deps.VisualAnalyzer, deps.Memograph,
			deps.VideoConfig, deps.WorkerConfig,
		),
	}
	if err := container.Validate(); err != nil {
		return nil, err
	}
	return container, nil
}

func (c *Container) Validate() error {
	if c == nil {
		return fmt.Errorf("service container is required")
	}
	if c.Health == nil {
		return fmt.Errorf("health service is required")
	}
	if c.Auth == nil {
		return fmt.Errorf("auth service is required")
	}
	if c.Voice == nil {
		return fmt.Errorf("voice service is required")
	}
	if c.Video == nil {
		return fmt.Errorf("video service is required")
	}
	return nil
}
