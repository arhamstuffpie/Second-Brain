package handler

import (
	"fmt"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

type Dependencies struct {
	HealthService service.HealthService
	AuthService   service.AuthService
	ModelService  service.ModelProfileService
	VoiceService  service.VoiceService
	VoiceConfig   config.VoiceConfig
	VideoService  service.VideoService
	VideoConfig   config.VideoConfig
	PersonService service.PersonService
	FaceConfig    config.FaceRecognitionConfig
	DebugService  service.PipelineDebugService
	DebugAdminID  string
}

type Container struct {
	Health HealthHandler
	Auth   AuthHandler
	Models ModelProfileHandler
	Voice  VoiceHandler
	Video  VideoHandler
	People PersonHandler
	Debug  PipelineDebugHandler
}

func NewContainer(deps Dependencies) (*Container, error) {
	if deps.HealthService == nil {
		return nil, fmt.Errorf("health service is required")
	}
	if deps.AuthService == nil {
		return nil, fmt.Errorf("auth service is required")
	}
	if deps.ModelService == nil {
		return nil, fmt.Errorf("model profile service is required")
	}
	if deps.VoiceService == nil {
		return nil, fmt.Errorf("voice service is required")
	}
	if deps.VideoService == nil {
		return nil, fmt.Errorf("video service is required")
	}
	if deps.PersonService == nil {
		return nil, fmt.Errorf("person service is required")
	}
	if deps.DebugAdminID != "" && deps.DebugService == nil {
		return nil, fmt.Errorf("pipeline debug service is required when debug is enabled")
	}

	container := &Container{
		Health: newHealthHandler(deps.HealthService),
		Auth:   newAuthHandler(deps.AuthService),
		Models: newModelProfileHandler(deps.ModelService),
		Voice: newVoiceHandler(
			deps.VoiceService, deps.VoiceConfig.MaxUploadBytes,
			deps.VoiceConfig.EnrollmentMaxUploadBytes,
		),
		Video:  newVideoHandler(deps.VideoService, deps.VideoConfig.MaxUploadBytes),
		People: newPersonHandler(deps.PersonService, deps.FaceConfig.MaxUploadBytes),
	}
	if deps.DebugAdminID != "" {
		container.Debug = newPipelineDebugHandler(
			deps.DebugService, deps.DebugAdminID,
			deps.FaceConfig.MaxUploadBytes,
			deps.VoiceConfig.EnrollmentMaxUploadBytes,
			deps.VideoConfig.MaxUploadBytes,
		)
	}
	if err := container.Validate(); err != nil {
		return nil, err
	}
	return container, nil
}

func (c *Container) Validate() error {
	if c == nil {
		return fmt.Errorf("handler container is required")
	}
	if c.Health == nil {
		return fmt.Errorf("health handler is required")
	}
	if c.Auth == nil {
		return fmt.Errorf("auth handler is required")
	}
	if c.Models == nil {
		return fmt.Errorf("model profile handler is required")
	}
	if c.Voice == nil {
		return fmt.Errorf("voice handler is required")
	}
	if c.Video == nil {
		return fmt.Errorf("video handler is required")
	}
	if c.People == nil {
		return fmt.Errorf("person handler is required")
	}
	return nil
}
