package worker

import (
	"context"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/rs/zerolog"
)

type DensePersonWorker struct {
	service service.DensePersonAnalysisService
	config  config.WorkerConfig
	logger  zerolog.Logger
}

func NewDensePersonWorker(
	analysisService service.DensePersonAnalysisService,
	cfg config.WorkerConfig,
	logger zerolog.Logger,
) *DensePersonWorker {
	return &DensePersonWorker{service: analysisService, config: cfg, logger: logger}
}

func (w *DensePersonWorker) Run(ctx context.Context) {
	if !w.config.Enabled || w.service == nil {
		w.logger.Info().Msg("dense person worker disabled")
		return
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	w.logger.Info().Msg("dense person worker started")
	defer w.logger.Info().Msg("dense person worker stopped")
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		processed, err := w.service.ProcessNextDensePersonAnalysis(ctx)
		if err != nil && ctx.Err() == nil {
			w.logger.Warn().Err(err).Msg("dense person analysis failed; retry state updated")
		}
		delay := time.Duration(0)
		if !processed {
			delay = w.config.PollInterval
		}
		timer.Reset(delay)
	}
}
