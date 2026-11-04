package repository

import (
	"context"

	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
)

type ChannelRepository interface {
	GetByCode(ctx context.Context, code string) (*model.Channel, error)
	GetByID(ctx context.Context, id string) (*model.Channel, error)
}

type TemplateRepository interface {
	GetByEventType(ctx context.Context, eventType string) (*model.Template, error)
	GetByEventTypeAndChannel(ctx context.Context, eventType string, channelID string) (*model.Template, error)
}

type PreferenceRepository interface {
	GetPreference(ctx context.Context, userID, channelID, eventType string) (*model.Preference, error)
	UpsertPreference(ctx context.Context, pref *model.Preference) error
}

type JobRepository interface {
	CreateJob(ctx context.Context, job *model.NotificationJob) error
	GetJobByEventID(ctx context.Context, eventID string) (*model.NotificationJob, error)
	GetJobByID(ctx context.Context, id string) (*model.NotificationJob, error)
	UpdateJobStatus(ctx context.Context, id string, status model.JobStatus, errMsg *string) error
	IncrementRetry(ctx context.Context, id string, errMsg string) error
	ListPendingJobs(ctx context.Context, limit int) ([]*model.NotificationJob, error)
}

type IdempotencyRepository interface {
	IsEventProcessed(ctx context.Context, eventID string) (bool, error)
	RecordProcessedEvent(ctx context.Context, event *model.ProcessedEvent) error
}

