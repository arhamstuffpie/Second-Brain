package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/rs/zerolog"
)

type densePersonAnalysisService struct {
	repository           DensePersonAnalysisRepository
	analyzer             DensePersonAnalyzer
	store                VideoStore
	profile              PersonTrackingProfile
	detectorModel        string
	embeddingModel       string
	configurationProfile string
	staleAfter           time.Duration
	logger               *zerolog.Logger
}

func newDensePersonAnalysisService(
	repository DensePersonAnalysisRepository,
	analyzer DensePersonAnalyzer,
	store VideoStore,
	personConfig config.PersonTrackingConfig,
	logger *zerolog.Logger,
) (*densePersonAnalysisService, error) {
	if repository == nil || analyzer == nil || store == nil {
		return nil, fmt.Errorf("dense person analysis dependencies are required")
	}
	profile := PersonTrackingProfile{
		FPS:                           personConfig.Profile.FPS,
		ConfirmationDetections:        personConfig.Profile.ConfirmationDetections,
		ConfirmationWindowFrames:      personConfig.Profile.ConfirmationWindowFrames,
		LostTimeoutSeconds:            personConfig.Profile.LostTimeoutSeconds,
		ReidentificationWindowSeconds: personConfig.Profile.ReidentificationWindowSeconds,
		HighConfidenceThreshold:       personConfig.Profile.HighConfidenceThreshold,
		LowConfidenceThreshold:        personConfig.Profile.LowConfidenceThreshold,
		IOUThreshold:                  personConfig.Profile.IOUThreshold,
		AppearanceThreshold:           personConfig.Profile.AppearanceThreshold,
		MaxGallerySamples:             personConfig.Profile.MaxGallerySamples,
	}
	encoded, err := json.Marshal(map[string]any{
		"stage":           "dense_person_tracking",
		"detector_model":  personConfig.DetectorModel,
		"embedding_model": personConfig.EmbeddingModel,
		"profile":         profile,
	})
	if err != nil {
		return nil, fmt.Errorf("encode person tracking configuration: %w", err)
	}
	return &densePersonAnalysisService{
		repository:           repository,
		analyzer:             analyzer,
		store:                store,
		profile:              profile,
		detectorModel:        personConfig.DetectorModel,
		embeddingModel:       personConfig.EmbeddingModel,
		configurationProfile: string(encoded),
		staleAfter:           2 * personConfig.Timeout,
		logger:               logger,
	}, nil
}

func (s *densePersonAnalysisService) ProcessNextDensePersonAnalysis(ctx context.Context) (bool, error) {
	job, found, err := s.repository.ClaimDensePersonAnalysis(
		ctx, s.configurationProfile, s.staleAfter,
	)
	if err != nil || !found {
		return found, err
	}

	content, err := s.store.Open(ctx, job.FilePath)
	if err == nil {
		analysis, analyzeErr := s.analyzer.AnalyzePeople(ctx, DensePersonAnalysisInput{
			RecordingID:       job.RecordingID,
			ProcessingVersion: job.ProcessingVersion,
			FileName:          job.FileName,
			MediaType:         job.MediaType,
			Video:             content,
			DetectorModel:     s.detectorModel,
			EmbeddingModel:    s.embeddingModel,
			Profile:           s.profile,
		})
		closeErr := content.Close()
		if analyzeErr != nil {
			err = analyzeErr
		} else if closeErr != nil {
			err = fmt.Errorf("close dense person source: %w", closeErr)
		} else {
			err = s.repository.CompleteDensePersonAnalysis(ctx, job, analysis)
		}
	}
	if err == nil {
		if s.logger != nil {
			s.logger.Info().Str("recording_id", job.RecordingID).Int64("job_id", job.ID).Msg("dense person analysis completed")
		}
		return true, nil
	}

	dead := job.Attempts >= job.MaxAttempts
	retryCtx := ctx
	var cancel context.CancelFunc
	if ctx.Err() != nil {
		retryCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	if retryErr := s.repository.RetryDensePersonAnalysis(
		retryCtx, job, err.Error(), time.Now().UTC().Add(retryDelayForError(err, job.Attempts)), dead,
	); retryErr != nil {
		return true, fmt.Errorf("%v; persist dense person retry: %w", err, retryErr)
	}
	return true, err
}

var _ DensePersonAnalysisService = (*densePersonAnalysisService)(nil)
