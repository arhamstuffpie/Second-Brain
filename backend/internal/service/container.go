package service

import (
	"fmt"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/rs/zerolog"
)

type Dependencies struct {
	HealthRepository    HealthRepository
	UserRepository      UserRepository
	ModelProfiles       ModelProfileRepository
	CredentialCipher    CredentialCipher
	JWT                 config.JWTConfig
	STTConfig           config.STTConfig
	VoiceRepository     VoiceRepository
	SpeakerProfiles     SpeakerProfileRepository
	SpeakerIdentifier   SpeakerIdentifier
	PersonRepository    PersonRepository
	FaceRecognizer      FaceRecognizer
	ActiveSpeaker       ActiveSpeakerDetector
	FaceStore           AudioStore
	VideoRepository     VideoRepository
	Transcriber         Transcriber
	SpeakerAttributor   SpeakerAttributor
	AudioStore          AudioStore
	EnrollmentStore     AudioStore
	AudioInspector      AudioInspector
	VideoStore          VideoStore
	MediaExtractor      MediaExtractor
	VisualAnalyzer      VisualAnalyzer
	Memograph           MemographClient
	VoiceConfig         config.VoiceConfig
	VideoConfig         config.VideoConfig
	FaceConfig          config.FaceRecognitionConfig
	ActiveSpeakerConfig config.ActiveSpeakerConfig
	WorkerConfig        config.WorkerConfig
	Logger              *zerolog.Logger
}

type Container struct {
	Health HealthService
	Auth   AuthService
	Models ModelProfileService
	Voice  VoiceService
	Video  VideoService
	People PersonService
}

func NewContainer(deps Dependencies) (*Container, error) {
	if deps.HealthRepository == nil {
		return nil, fmt.Errorf("health repository is required")
	}
	if deps.UserRepository == nil {
		return nil, fmt.Errorf("user repository is required")
	}
	if deps.ModelProfiles == nil || deps.CredentialCipher == nil {
		return nil, fmt.Errorf("model profile dependencies are required")
	}
	if deps.VoiceRepository == nil || deps.SpeakerProfiles == nil || deps.Transcriber == nil || deps.SpeakerAttributor == nil ||
		deps.AudioStore == nil || deps.EnrollmentStore == nil || deps.AudioInspector == nil ||
		deps.Memograph == nil {
		return nil, fmt.Errorf("voice service dependencies are required")
	}
	if deps.VideoRepository == nil || deps.VideoStore == nil ||
		deps.MediaExtractor == nil || deps.VisualAnalyzer == nil {
		return nil, fmt.Errorf("video service dependencies are required")
	}
	if deps.PersonRepository == nil || deps.FaceStore == nil {
		return nil, fmt.Errorf("person identity dependencies are required")
	}
	if len(deps.JWT.Secret) < 32 || deps.JWT.Issuer == "" || deps.JWT.AccessTokenTTL <= 0 {
		return nil, fmt.Errorf("valid JWT configuration is required")
	}

	voice := newVoiceService(
		deps.VoiceRepository, deps.Transcriber, deps.SpeakerAttributor,
		deps.AudioStore, deps.EnrollmentStore, deps.AudioInspector, deps.Memograph,
		deps.VoiceConfig, deps.WorkerConfig,
	)
	voice.speakerProfiles = deps.SpeakerProfiles
	voice.speakerIdentifier = deps.SpeakerIdentifier
	video := newVideoService(
		deps.VideoRepository, deps.VoiceRepository, deps.Transcriber,
		deps.SpeakerAttributor, deps.EnrollmentStore, deps.VideoStore,
		deps.MediaExtractor, deps.VisualAnalyzer, deps.Memograph,
		deps.VideoConfig, deps.WorkerConfig,
	)
	video.speakerProfiles = deps.SpeakerProfiles
	video.speakerIdentifier = deps.SpeakerIdentifier
	video.faceIdentifier = NewVideoFaceIdentifier(
		deps.PersonRepository, deps.FaceRecognizer, deps.FaceConfig,
	)
	video.personRepository = deps.PersonRepository
	video.activeSpeaker = deps.ActiveSpeaker
	video.faceConfig = deps.FaceConfig
	video.activeSpeakerConfig = deps.ActiveSpeakerConfig
	video.logger = deps.Logger
	if identifier, ok := video.faceIdentifier.(*videoFaceIdentifier); ok {
		identifier.logger = deps.Logger
	}

	container := &Container{
		Health: newHealthService(deps.HealthRepository),
		Auth:   newAuthService(deps.UserRepository, deps.JWT),
		Models: newModelProfileService(deps.ModelProfiles, deps.CredentialCipher, deps.STTConfig),
		Voice:  voice,
		Video:  video,
		People: newPersonService(deps.PersonRepository, deps.FaceRecognizer, deps.FaceStore, deps.FaceConfig),
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
	if c.Models == nil {
		return fmt.Errorf("model profile service is required")
	}
	if c.Voice == nil {
		return fmt.Errorf("voice service is required")
	}
	if c.Video == nil {
		return fmt.Errorf("video service is required")
	}
	if c.People == nil {
		return fmt.Errorf("person service is required")
	}
	return nil
}
