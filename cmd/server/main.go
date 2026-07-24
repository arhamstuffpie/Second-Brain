package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arham/ai-second-brain/internal/audio"
	"github.com/arham/ai-second-brain/internal/config"
	internaldb "github.com/arham/ai-second-brain/internal/db"
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/arham/ai-second-brain/internal/http/middleware"
	"github.com/arham/ai-second-brain/internal/http/router"
	internallogger "github.com/arham/ai-second-brain/internal/logger"
	"github.com/arham/ai-second-brain/internal/memograph"
	"github.com/arham/ai-second-brain/internal/repository"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/arham/ai-second-brain/internal/stt"
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
	audioStore, err := audio.NewLocalStore(cfg.Voice.StorageDir, cfg.Voice.MaxUploadBytes)
	if err != nil {
		return fmt.Errorf("construct audio store: %w", err)
	}
	var transcriber service.Transcriber
	switch cfg.STT.Provider {
	case "openai":
		transcriber, err = stt.NewOpenAI(cfg.STT)
		if err != nil {
			return fmt.Errorf("construct OpenAI transcriber: %w", err)
		}
	default:
		transcriber = stt.NewMock()
	}
	memographClient := memograph.NewClient(cfg.Memograph)
	services, err := service.NewContainer(service.Dependencies{
		HealthRepository: repositories.Health,
		UserRepository:   repositories.User,
		VoiceRepository:  repositories.Voice,
		Transcriber:      transcriber,
		AudioStore:       audioStore,
		Memograph:        memographClient,
		VoiceConfig:      cfg.Voice,
		WorkerConfig:     cfg.Worker,
		JWT:              cfg.JWT,
	})
	if err != nil {
		return fmt.Errorf("construct services: %w", err)
	}
	handlers, err := handler.NewContainer(handler.Dependencies{
		HealthService: services.Health,
		AuthService:   services.Auth,
		VoiceService:  services.Voice,
		VoiceConfig:   cfg.Voice,
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
	voiceWorkerDone := make(chan struct{})
	go func() {
		defer close(voiceWorkerDone)
		voiceWorker.Run(rootCtx)
	}()
	defer func() {
		stopSignals()
		select {
		case <-voiceWorkerDone:
		case <-time.After(cfg.HTTP.ShutdownTimeout):
			appLogger.Warn().Msg("voice worker did not stop before shutdown timeout")
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

func loadDotEnv(filename string) error {
	err := godotenv.Load(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
