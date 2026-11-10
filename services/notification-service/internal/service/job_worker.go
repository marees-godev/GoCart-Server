package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/provider"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
)

type JobWorker struct {
	jobRepo  repository.JobRepository
	registry provider.Registry
	logger   *slog.Logger
	interval time.Duration
}

func NewJobWorker(jobRepo repository.JobRepository, registry provider.Registry, logger *slog.Logger, interval time.Duration) *JobWorker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if registry == nil {
		registry = provider.NewDefaultRegistry()
	}
	return &JobWorker{
		jobRepo:  jobRepo,
		registry: registry,
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
	channel := strings.TrimSpace(job.ChannelCode)
	if channel == "" {
		channel = "EMAIL"
	}

	prov, err := w.registry.Get(channel)
	if err != nil {
		errMsg := err.Error()
		w.logger.ErrorContext(ctx, "Unsupported notification channel, failing job",
			"job_id", job.ID,
			"channel", channel,
			"error", errMsg,
		)
		_ = w.jobRepo.UpdateJobStatus(ctx, job.ID, model.JobStatusFailed, &errMsg)
		return
	}

	var userID string
	if job.UserID != nil {
		userID = *job.UserID
	}

	notif := provider.Notification{
		ID:        job.ID,
		EventID:   job.EventID,
		UserID:    userID,
		Channel:   channel,
		Recipient: job.Recipient,
		Subject:   job.Subject,
		Content:   job.Content,
	}

	w.logger.InfoContext(ctx, "Delivering notification job via provider",
		"job_id", job.ID,
		"channel", channel,
		"provider", prov.ProviderName(),
		"recipient", job.Recipient,
	)

	if err := prov.Send(ctx, notif); err != nil {
		errMsg := err.Error()
		w.logger.ErrorContext(ctx, "Provider notification delivery failed",
			"job_id", job.ID,
			"channel", channel,
			"provider", prov.ProviderName(),
			"error", errMsg,
		)
		_ = w.jobRepo.IncrementRetry(ctx, job.ID, errMsg)
		return
	}

	if err := w.jobRepo.UpdateJobStatus(ctx, job.ID, model.JobStatusCompleted, nil); err != nil {
		w.logger.ErrorContext(ctx, "failed to update job to completed, retrying", "job_id", job.ID, "error", err)
		_ = w.jobRepo.IncrementRetry(ctx, job.ID, err.Error())
	}
}
