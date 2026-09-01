package worker

import (
	"context"
	"sync"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/rs/zerolog"
)

type VideoWorker struct {
	service service.VideoService
	config  config.WorkerConfig
	logger  zerolog.Logger
}

func NewVideoWorker(
	videoService service.VideoService,
	cfg config.WorkerConfig,
	logger zerolog.Logger,
) *VideoWorker {
	return &VideoWorker{service: videoService, config: cfg, logger: logger}
}

func (w *VideoWorker) Run(ctx context.Context) {
	if !w.config.Enabled {
		w.logger.Info().Msg("video worker disabled")
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
	w.logger.Info().Int("concurrency", w.config.Concurrency).Msg("video worker started")
	group.Wait()
	w.logger.Info().Msg("video worker stopped")
}

func (w *VideoWorker) runOne(ctx context.Context, workerID int) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		processed, err := w.service.ProcessNextVideoJob(ctx)
		if err == nil && !processed {
			// ponytail: identity jobs share this worker; split the queue only if sustained video traffic causes starvation.
			processed, err = w.service.ProcessNextIdentityJob(ctx)
		}
		if err != nil && ctx.Err() == nil {
			w.logger.Warn().Err(err).Int("worker_id", workerID).
				Msg("video job failed; retry state updated")
		}
		delay := time.Duration(0)
		if !processed {
			delay = w.config.PollInterval
		}
		timer.Reset(delay)
	}
}
