package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/provider"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
)

func TestJobWorker_ProcessSingleJob_Success(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	mockRepo := &mockJobRepo{
		jobs:          make(map[string]*model.NotificationJob),
		jobsByEventID: make(map[string]*model.NotificationJob),
	}

	reg := provider.NewRegistry()
	mockEmail := provider.NewMockEmailProvider(1 * time.Second)
	reg.Register("EMAIL", mockEmail)

	job := &model.NotificationJob{
		ID:          uuid.NewString(),
		EventID:     "evt-100",
		ChannelCode: "EMAIL",
		Recipient:   "user@example.com",
		Subject:     "Order Placed",
		Content:     "<p>Thank you for your order</p>",
		Status:      model.JobStatusPending,
		MaxRetries:  3,
	}
	_ = mockRepo.CreateJob(ctx, job)

	worker := service.NewJobWorker(mockRepo, reg, logger, 50*time.Millisecond)

	wCtx, cancel := context.WithTimeout(ctx, 120*time.Millisecond)
	defer cancel()

	go worker.Start(wCtx)
	time.Sleep(100 * time.Millisecond)

	updatedJob, err := mockRepo.GetJobByID(ctx, job.ID)
	if err != nil || updatedJob == nil {
		t.Fatalf("failed retrieving updated job: %v", err)
	}

	if updatedJob.Status != model.JobStatusCompleted {
		t.Errorf("expected job status COMPLETED, got: %s", updatedJob.Status)
	}

	sentList := mockEmail.SentNotifications()
	if len(sentList) != 1 {
		t.Fatalf("expected 1 sent email, got: %d", len(sentList))
	}
	if sentList[0].Recipient != "user@example.com" {
		t.Errorf("expected recipient user@example.com, got: %s", sentList[0].Recipient)
	}
}

func TestJobWorker_ProcessSingleJob_FailureAndRetry(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	mockRepo := &mockJobRepo{
		jobs:          make(map[string]*model.NotificationJob),
		jobsByEventID: make(map[string]*model.NotificationJob),
	}

	reg := provider.NewRegistry()
	mockEmail := provider.NewMockEmailProvider(1 * time.Second)
	mockEmail.SetFailNext(errors.New("simulated SMTP connection error"))
	reg.Register("EMAIL", mockEmail)

	job := &model.NotificationJob{
		ID:          uuid.NewString(),
		EventID:     "evt-err-1",
		ChannelCode: "EMAIL",
		Recipient:   "user@example.com",
		Subject:     "Retry Test",
		Content:     "Content",
		Status:      model.JobStatusPending,
		RetryCount:  0,
		MaxRetries:  3,
	}
	_ = mockRepo.CreateJob(ctx, job)

	worker := service.NewJobWorker(mockRepo, reg, logger, 50*time.Millisecond)

	wCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	go worker.Start(wCtx)
	time.Sleep(90 * time.Millisecond)

	updatedJob, _ := mockRepo.GetJobByID(ctx, job.ID)
	if updatedJob.RetryCount != 1 {
		t.Errorf("expected retry count 1, got: %d", updatedJob.RetryCount)
	}
}

func TestJobWorker_ProcessSingleJob_MaxRetriesExceeded_Fails(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	mockRepo := &mockJobRepo{
		jobs:          make(map[string]*model.NotificationJob),
		jobsByEventID: make(map[string]*model.NotificationJob),
	}

	reg := provider.NewRegistry()
	mockEmail := provider.NewMockEmailProvider(1 * time.Second)
	mockEmail.SetFailNext(errors.New("permanent failure"))
	reg.Register("EMAIL", mockEmail)

	job := &model.NotificationJob{
		ID:          uuid.NewString(),
		EventID:     "evt-fail-final",
		ChannelCode: "EMAIL",
		Recipient:   "user@example.com",
		Subject:     "Final Fail Test",
		Content:     "Content",
		Status:      model.JobStatusPending,
		RetryCount:  2,
		MaxRetries:  3,
	}
	_ = mockRepo.CreateJob(ctx, job)

	worker := service.NewJobWorker(mockRepo, reg, logger, 50*time.Millisecond)

	wCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	go worker.Start(wCtx)
	time.Sleep(90 * time.Millisecond)

	updatedJob, _ := mockRepo.GetJobByID(ctx, job.ID)
	if updatedJob.RetryCount < 3 {
		t.Errorf("expected retry count >= 3, got: %d", updatedJob.RetryCount)
	}
}

func TestJobWorker_ProcessSingleJob_UnsupportedChannel(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	mockRepo := &mockJobRepo{
		jobs:          make(map[string]*model.NotificationJob),
		jobsByEventID: make(map[string]*model.NotificationJob),
	}

	reg := provider.NewRegistry() // Empty registry without CARRIER_PIGEON

	job := &model.NotificationJob{
		ID:          uuid.NewString(),
		EventID:     "evt-unknown-channel",
		ChannelCode: "CARRIER_PIGEON",
		Recipient:   "pigeon-1",
		Subject:     "Unknown Channel",
		Content:     "Content",
		Status:      model.JobStatusPending,
		MaxRetries:  3,
	}
	_ = mockRepo.CreateJob(ctx, job)

	worker := service.NewJobWorker(mockRepo, reg, logger, 50*time.Millisecond)

	wCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	go worker.Start(wCtx)
	time.Sleep(90 * time.Millisecond)

	updatedJob, _ := mockRepo.GetJobByID(ctx, job.ID)
	if updatedJob.Status != model.JobStatusFailed {
		t.Errorf("expected job status FAILED for unsupported channel, got: %s", updatedJob.Status)
	}
}

func TestJobWorker_SwappableProvider_Integration(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	mockRepo := &mockJobRepo{
		jobs:          make(map[string]*model.NotificationJob),
		jobsByEventID: make(map[string]*model.NotificationJob),
	}

	reg := provider.NewRegistry()
	provider1 := provider.NewMockEmailProvider(1 * time.Second)
	reg.Register("EMAIL", provider1)

	worker := service.NewJobWorker(mockRepo, reg, logger, 50*time.Millisecond)

	// Send with provider1
	job1 := &model.NotificationJob{
		ID:          uuid.NewString(),
		EventID:     "evt-swap-1",
		ChannelCode: "EMAIL",
		Recipient:   "user1@example.com",
		Subject:     "Sub 1",
		Content:     "Body 1",
		Status:      model.JobStatusPending,
		MaxRetries:  3,
	}
	_ = mockRepo.CreateJob(ctx, job1)

	wCtx1, cancel1 := context.WithTimeout(ctx, 100*time.Millisecond)
	go worker.Start(wCtx1)
	time.Sleep(90 * time.Millisecond)
	cancel1()

	if len(provider1.SentNotifications()) != 1 {
		t.Fatalf("expected provider1 to receive 1 email, got: %d", len(provider1.SentNotifications()))
	}

	// Swap provider2 in registry dynamically
	provider2 := provider.NewMockEmailProvider(1 * time.Second)
	reg.Register("EMAIL", provider2)

	job2 := &model.NotificationJob{
		ID:          uuid.NewString(),
		EventID:     "evt-swap-2",
		ChannelCode: "EMAIL",
		Recipient:   "user2@example.com",
		Subject:     "Sub 2",
		Content:     "Body 2",
		Status:      model.JobStatusPending,
		MaxRetries:  3,
	}
	_ = mockRepo.CreateJob(ctx, job2)

	wCtx2, cancel2 := context.WithTimeout(ctx, 100*time.Millisecond)
	go worker.Start(wCtx2)
	time.Sleep(90 * time.Millisecond)
	cancel2()

	if len(provider2.SentNotifications()) != 1 {
		t.Fatalf("expected provider2 to receive 1 email after swap, got: %d", len(provider2.SentNotifications()))
	}
}

