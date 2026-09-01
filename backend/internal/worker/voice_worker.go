package worker

import (
	"context"
	"sync"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/rs/zerolog"
)

type VoiceWorker struct {
	service service.VoiceService
	config  config.WorkerConfig
	logger  zerolog.Logger
}

func NewVoiceWorker(voiceService service.VoiceService, cfg config.WorkerConfig, logger zerolog.Logger) *VoiceWorker {
	return &VoiceWorker{service: voiceService, config: cfg, logger: logger}
}

func (w *VoiceWorker) Run(ctx context.Context) {
	if !w.config.Enabled {
		w.logger.Info().Msg("voice worker disabled")
		return
	}
	var group sync.WaitGroup
	for index := 0; index < w.config.Concurrency; index++ {
		group.Add(1)
		go func(workerID int) {
			defer group.Done()
			w.runOne(ctx, workerID)
		}(index + 1)
	}
	w.logger.Info().Int("concurrency", w.config.Concurrency).Msg("voice worker started")
	group.Wait()
	w.logger.Info().Msg("voice worker stopped")
}

func (w *VoiceWorker) runOne(ctx context.Context, workerID int) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		processed, err := w.service.ProcessNextJob(ctx)
		if err != nil && ctx.Err() == nil {
			w.logger.Warn().Err(err).Int("worker_id", workerID).Msg("voice job failed; retry state updated")
		}
		delay := time.Duration(0)
		if !processed {
			delay = w.config.PollInterval
		}
		timer.Reset(delay)
	}
}
