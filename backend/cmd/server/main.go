package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	active_speaker "github.com/arham/ai-second-brain/internal/active_speaker"
	"github.com/arham/ai-second-brain/internal/audio"
	"github.com/arham/ai-second-brain/internal/config"
	internaldb "github.com/arham/ai-second-brain/internal/db"
	"github.com/arham/ai-second-brain/internal/face"
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/arham/ai-second-brain/internal/http/middleware"
	"github.com/arham/ai-second-brain/internal/http/router"
	internallogger "github.com/arham/ai-second-brain/internal/logger"
	"github.com/arham/ai-second-brain/internal/media"
	"github.com/arham/ai-second-brain/internal/memograph"
	person_tracking "github.com/arham/ai-second-brain/internal/person_tracking"
	"github.com/arham/ai-second-brain/internal/repository"
	"github.com/arham/ai-second-brain/internal/secrets"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/arham/ai-second-brain/internal/speaker"
	"github.com/arham/ai-second-brain/internal/stt"
	"github.com/arham/ai-second-brain/internal/vision"
	"github.com/arham/ai-second-brain/internal/worker"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

func main() {
	bootstrapLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	if err := run(); err != nil {
		bootstrapLogger.Error().Err(err).Msg("server stopped")
		os.Exit(1)
	}
}

func newStore(ctx context.Context, cfg config.Config, localDir string, maxBytes int64) (service.AudioStore, error) {
	if cfg.Storage.S3Bucket != "" && os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
		return audio.NewS3Store(ctx, cfg.Storage.S3Bucket, cfg.Storage.S3Prefix, cfg.Storage.S3Region, maxBytes)
	}
	return audio.NewLocalStore(localDir, maxBytes)
}

func run() error {
	// This is the only composition root: each container is constructed once and
	// passed down to the next layer.
	if err := loadDotEnv(".env"); err != nil {
		return fmt.Errorf("load .env: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	appLogger, err := internallogger.New(cfg.Log, cfg.Environment)
	if err != nil {
		return err
	}

	rootCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	database, err := internaldb.NewPostgres(rootCtx, cfg.Database)
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			appLogger.Error().Err(err).Msg("close database")
		}
	}()

	repositories, err := repository.NewContainer(database)
	if err != nil {
		return fmt.Errorf("construct repositories: %w", err)
	}
	audioStore, err := newStore(rootCtx, cfg, cfg.Voice.StorageDir, cfg.Voice.MaxUploadBytes)
	if err != nil {
		return fmt.Errorf("construct audio store: %w", err)
	}
	enrollmentStore, err := newStore(rootCtx, cfg, cfg.Voice.EnrollmentStorageDir, cfg.Voice.EnrollmentMaxUploadBytes)
	if err != nil {
		return fmt.Errorf("construct voice enrollment store: %w", err)
	}
	faceStore, err := newStore(rootCtx, cfg, cfg.Face.StorageDir, cfg.Face.MaxUploadBytes)
	if err != nil {
		return fmt.Errorf("construct face enrollment store: %w", err)
	}
	audioInspector, err := audio.NewFFprobeInspector(
		cfg.Voice.FFprobePath, cfg.Voice.InspectionTimeout,
	)
	if err != nil {
		return fmt.Errorf("construct voice audio inspector: %w", err)
	}
	videoStore, err := newStore(rootCtx, cfg, cfg.Video.StorageDir, cfg.Video.MaxUploadBytes)
	if err != nil {
		return fmt.Errorf("construct video store: %w", err)
	}
	mediaExtractor, err := media.NewFFmpegExtractor(cfg.Video)
	if err != nil {
		return fmt.Errorf("construct media extractor: %w", err)
	}
	credentialCipher, err := secrets.NewCipher(cfg.Models.CredentialKey)
	if err != nil {
		return fmt.Errorf("construct model credential cipher: %w", err)
	}
	var defaultTranscriber service.Transcriber
	switch cfg.STT.Provider {
	case "mock":
		defaultTranscriber = stt.NewMock()
	default:
		defaultTranscriber, err = stt.NewCompatible(cfg.STT.Provider, cfg.STT)
		if err != nil {
			return fmt.Errorf("construct %s transcriber: %w", cfg.STT.Provider, err)
		}
	}
	transcriber, err := stt.NewProfileAware(
		defaultTranscriber, repositories.Models, credentialCipher, cfg.STT,
	)
	if err != nil {
		return fmt.Errorf("construct profile-aware transcriber: %w", err)
	}
	var speakerEmbedder service.SpeakerEmbedder
	var speakerIdentifier service.SpeakerIdentifier
	if cfg.Speaker.Provider != "disabled" {
		embedder, embedderErr := speaker.NewHTTPEmbedder(cfg.Speaker)
		if embedderErr != nil {
			return fmt.Errorf("construct speaker embedder: %w", embedderErr)
		}
		speakerEmbedder = embedder
		speakerIdentifier, err = service.NewPersistentSpeakerIdentifier(
			repositories.Speakers, embedder, mediaExtractor, enrollmentStore, cfg.Speaker,
		)
		if err != nil {
			return fmt.Errorf("construct persistent speaker identifier: %w", err)
		}
	}
	var visualAnalyzer service.VisualAnalyzer
	switch cfg.Vision.Provider {
	case "openai":
		visualAnalyzer, err = vision.NewOpenAI(cfg.Vision)
		if err != nil {
			return fmt.Errorf("construct OpenAI visual analyzer: %w", err)
		}
	default:
		visualAnalyzer = vision.NewMock()
	}
	var faceRecognizer service.FaceRecognizer
	if cfg.Face.Provider != "disabled" {
		faceRecognizer, err = face.NewHTTPRecognizer(cfg.Face)
		if err != nil {
			return fmt.Errorf("construct face recognizer: %w", err)
		}
		validationCtx, cancel := context.WithTimeout(rootCtx, cfg.Face.Timeout)
		_, err = faceRecognizer.Validate(validationCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("validate face recognizer: %w", err)
		}
	}
	var activeSpeaker service.ActiveSpeakerDetector
	if cfg.ActiveSpeaker.Provider != "disabled" {
		activeSpeaker, err = active_speaker.NewHTTPDetector(cfg.ActiveSpeaker)
		if err != nil {
			return fmt.Errorf("construct active-speaker detector: %w", err)
		}
		validationCtx, cancel := context.WithTimeout(rootCtx, cfg.ActiveSpeaker.Timeout)
		_, err = activeSpeaker.Validate(validationCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("validate active-speaker detector: %w", err)
		}
	}
	var densePersonAnalyzer service.DensePersonAnalyzer
	if cfg.PersonTracking.Provider != "disabled" {
		densePersonAnalyzer, err = person_tracking.NewHTTPAnalyzer(
			cfg.PersonTracking.BaseURL,
			cfg.PersonTracking.APIKey,
			cfg.PersonTracking.DetectorModel,
			cfg.PersonTracking.EmbeddingModel,
			cfg.PersonTracking.Provider,
			cfg.PersonTracking.Timeout,
		)
		if err != nil {
			return fmt.Errorf("construct dense person analyzer: %w", err)
		}
	}
	appLogger.Info().
		Str("stt_provider", transcriber.Provider()).
		Str("stt_model", transcriber.Model()).
		Str("speaker_embedding_provider", cfg.Speaker.Provider).
		Str("speaker_embedding_model", cfg.Speaker.Model).
		Str("face_recognition_provider", cfg.Face.Provider).
		Str("face_recognition_model", cfg.Face.Model).
		Str("person_analyzer_provider", cfg.PersonTracking.Provider).
		Str("person_analyzer_detector_model", cfg.PersonTracking.DetectorModel).
		Str("person_analyzer_embedding_model", cfg.PersonTracking.EmbeddingModel).
		Str("active_speaker_provider", cfg.ActiveSpeaker.Provider).
		Str("active_speaker_model", cfg.ActiveSpeaker.Model).
		Str("vision_provider", visualAnalyzer.Provider()).
		Str("vision_model", visualAnalyzer.Model()).
		Msg("media analysis providers configured")
	memographClient := memograph.NewClient(cfg.Memograph)
	services, err := service.NewContainer(service.Dependencies{
		HealthRepository:              repositories.Health,
		UserRepository:                repositories.User,
		ModelProfiles:                 repositories.Models,
		CredentialCipher:              credentialCipher,
		STTConfig:                     cfg.STT,
		VoiceRepository:               repositories.Voice,
		SpeakerProfiles:               repositories.Speakers,
		SpeakerEmbedder:               speakerEmbedder,
		SpeakerIdentifier:             speakerIdentifier,
		PersonRepository:              repositories.People,
		DensePersonRepository:         repositories.DensePeople,
		ActiveSpeakerFusionRepository: repositories.ActiveSpeakerFusion,
		DensePersonAnalyzer:           densePersonAnalyzer,
		FaceRecognizer:                faceRecognizer,
		ActiveSpeaker:                 activeSpeaker,
		FaceStore:                     faceStore,
		VideoRepository:               repositories.Video,
		Transcriber:                   transcriber,
		SpeakerAttributor:             stt.NewReferenceAttributor(),
		AudioStore:                    audioStore,
		EnrollmentStore:               enrollmentStore,
		AudioInspector:                audioInspector,
		VideoStore:                    videoStore,
		MediaExtractor:                mediaExtractor,
		VisualAnalyzer:                visualAnalyzer,
		Memograph:                     memographClient,
		VoiceConfig:                   cfg.Voice,
		VideoConfig:                   cfg.Video,
		FaceConfig:                    cfg.Face,
		PersonTrackingConfig:          cfg.PersonTracking,
		ActiveSpeakerConfig:           cfg.ActiveSpeaker,
		WorkerConfig:                  cfg.Worker,
		Logger:                        &appLogger,
		JWT:                           cfg.JWT,
	})
	if err != nil {
		return fmt.Errorf("construct services: %w", err)
	}
	debugAdminID := ""
	if cfg.Debug.Enabled {
		debugAdmin, debugErr := ensurePipelineDebugAdmin(rootCtx, services.Auth, cfg.Debug)
		if debugErr != nil {
			return fmt.Errorf("prepare pipeline debug admin: %w", debugErr)
		}
		debugAdminID = debugAdmin.User.ID
		appLogger.Info().Str("email", debugAdmin.User.Email).Msg("pipeline debug dashboard enabled")
	}
	handlers, err := handler.NewContainer(handler.Dependencies{
		HealthService: services.Health,
		AuthService:   services.Auth,
		ModelService:  services.Models,
		VoiceService:  services.Voice,
		VoiceConfig:   cfg.Voice,
		VideoService:  services.Video,
		VideoConfig:   cfg.Video,
		PersonService: services.People,
		FaceConfig:    cfg.Face,
		DebugService:  services.Debug,
		DebugAdminID:  debugAdminID,
	})
	if err != nil {
		return fmt.Errorf("construct handlers: %w", err)
	}
	middlewares, err := middleware.NewContainer(cfg)
	if err != nil {
		return fmt.Errorf("construct middleware: %w", err)
	}
	httpRouter, err := router.New(router.Dependencies{
		Config: cfg, Logger: appLogger, Handlers: handlers, Middleware: middlewares,
	})
	if err != nil {
		return fmt.Errorf("construct router: %w", err)
	}

	server := &http.Server{
		Addr:              cfg.HTTP.Address(),
		Handler:           httpRouter,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		MaxHeaderBytes:    cfg.HTTP.MaxHeaderBytes,
		ErrorLog:          log.New(appLogger, "", 0),
	}

	serverErrors := make(chan error, 1)
	voiceWorker := worker.NewVoiceWorker(services.Voice, cfg.Worker, appLogger)
	videoWorker := worker.NewVideoWorker(services.Video, cfg.Worker, appLogger)
	densePersonWorker := worker.NewDensePersonWorker(services.DensePeople, cfg.Worker, appLogger)
	activeSpeakerFusionWorker := worker.NewActiveSpeakerFusionWorker(services.ActiveSpeakerFusion, cfg.Worker, appLogger)
	workersDone := make(chan struct{})
	go func() {
		defer close(workersDone)
		var group sync.WaitGroup
		group.Add(4)
		go func() {
			defer group.Done()
			voiceWorker.Run(rootCtx)
		}()
		go func() {
			defer group.Done()
			videoWorker.Run(rootCtx)
		}()
		go func() {
			defer group.Done()
			densePersonWorker.Run(rootCtx)
		}()
		go func() {
			defer group.Done()
			activeSpeakerFusionWorker.Run(rootCtx)
		}()
		group.Wait()
	}()
	defer func() {
		stopSignals()
		select {
		case <-workersDone:
		case <-time.After(cfg.HTTP.ShutdownTimeout):
			appLogger.Warn().Msg("media workers did not stop before shutdown timeout")
		}
	}()
	go func() {
		appLogger.Info().Str("address", server.Addr).Msg("http server started")
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-rootCtx.Done():
		appLogger.Info().Msg("shutdown signal received")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}

	appLogger.Info().Msg("http server stopped gracefully")
	return nil
}

func ensurePipelineDebugAdmin(ctx context.Context, auth service.AuthService, cfg config.DebugConfig) (service.AuthResult, error) {
	result, err := auth.Login(ctx, cfg.AdminEmail, cfg.AdminPassword)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, service.ErrUnauthorized) {
		return service.AuthResult{}, err
	}
	return auth.Signup(ctx, cfg.AdminEmail, cfg.AdminPassword)
}

func loadDotEnv(filename string) error {
	err := godotenv.Load(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
