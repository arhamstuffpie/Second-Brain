package worker

import (
	"context"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/rs/zerolog"
)

type ActiveSpeakerFusionWorker struct {
	service service.ActiveSpeakerFusionService
	config  config.WorkerConfig
	logger  zerolog.Logger
}

func NewActiveSpeakerFusionWorker(
	fusionService service.ActiveSpeakerFusionService,
	cfg config.WorkerConfig,
	logger zerolog.Logger,
) *ActiveSpeakerFusionWorker {
	return &ActiveSpeakerFusionWorker{service: fusionService, config: cfg, logger: logger}
}

func (w *ActiveSpeakerFusionWorker) Run(ctx context.Context) {
	if !w.config.Enabled || w.service == nil {
		w.logger.Info().Msg("active-speaker fusion worker disabled")
		return
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	w.logger.Info().Msg("active-speaker fusion worker started")
	defer w.logger.Info().Msg("active-speaker fusion worker stopped")
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		processed, err := w.service.ProcessNextActiveSpeakerFusion(ctx)
		if err != nil && ctx.Err() == nil {
			w.logger.Warn().Err(err).Msg("active-speaker fusion failed; retry state updated")
		}
		delay := time.Duration(0)
		if !processed {
			delay = w.config.PollInterval
		}
		timer.Reset(delay)
	}
}
