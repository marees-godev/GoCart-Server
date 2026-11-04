package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/service"
)

type RetentionWorker struct {
	deliveryService service.DeliveryService
	interval        time.Duration
	retentionPeriod time.Duration
	done            chan struct{}
}

func NewRetentionWorker(deliveryService service.DeliveryService, interval, retentionPeriod time.Duration) *RetentionWorker {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	if retentionPeriod <= 0 {
		retentionPeriod = 30 * 24 * time.Hour
	}
	return &RetentionWorker{
		deliveryService: deliveryService,
		interval:        interval,
		retentionPeriod: retentionPeriod,
		done:            make(chan struct{}),
	}
}

func (w *RetentionWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	go func() {
		defer ticker.Stop()
		defer close(w.done)
		slog.InfoContext(ctx, "delivery partner retention worker started", "interval", w.interval, "retention_period", w.retentionPeriod)

		if count, err := w.deliveryService.ProcessExpiredDeactivations(ctx, w.retentionPeriod); err != nil {
			slog.ErrorContext(ctx, "failed to process expired partner deactivations on worker startup", "error", err)
		} else if count > 0 {
			slog.InfoContext(ctx, "processed expired partner deactivations on worker startup", "count", count)
		}

		for {
			select {
			case <-ctx.Done():
				slog.InfoContext(ctx, "delivery partner retention worker stopped")
				return
			case <-ticker.C:
				if count, err := w.deliveryService.ProcessExpiredDeactivations(ctx, w.retentionPeriod); err != nil {
					slog.ErrorContext(ctx, "failed to process expired partner deactivations", "error", err)
				} else if count > 0 {
					slog.InfoContext(ctx, "processed expired partner deactivations in worker tick", "count", count)
				}
			}
		}
	}()
}

func (w *RetentionWorker) Done() <-chan struct{} {
	return w.done
}

