package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
)

type JobWorker struct {
	jobRepo  repository.JobRepository
	logger   *slog.Logger
	interval time.Duration
}

func NewJobWorker(jobRepo repository.JobRepository, logger *slog.Logger, interval time.Duration) *JobWorker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &JobWorker{
		jobRepo:  jobRepo,
		logger:   logger,
		interval: interval,
	}
}

func (w *JobWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *JobWorker) processBatch(ctx context.Context) {
	jobs, err := w.jobRepo.ClaimPendingJobs(ctx, 50)
	if err != nil {
		w.logger.ErrorContext(ctx, "failed claiming pending notification jobs", "error", err)
		return
	}

	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		w.processSingleJob(ctx, job)
	}
}

func (w *JobWorker) processSingleJob(ctx context.Context, job *model.NotificationJob) {
	// Simulated notification delivery (e.g., SMTP / SMS gateway / Push)
	w.logger.InfoContext(ctx, "Delivering notification job",
		"job_id", job.ID,
		"channel", job.ChannelCode,
		"recipient", job.Recipient,
		"subject", job.Subject,
	)

	err := w.jobRepo.UpdateJobStatus(ctx, job.ID, model.JobStatusCompleted, nil)
	if err != nil {
		w.logger.ErrorContext(ctx, "failed to update job to completed, retrying", "job_id", job.ID, "error", err)
		_ = w.jobRepo.IncrementRetry(ctx, job.ID, err.Error())
	}
}
