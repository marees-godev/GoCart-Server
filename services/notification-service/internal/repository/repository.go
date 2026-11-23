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
	ClaimPendingJobs(ctx context.Context, limit int) ([]*model.NotificationJob, error)
}

type IdempotencyRepository interface {
	IsEventProcessed(ctx context.Context, eventID string) (bool, error)
	RecordProcessedEvent(ctx context.Context, event *model.ProcessedEvent) error
}

type PushSubscriptionRepository interface {
	UpsertSubscription(ctx context.Context, sub *model.PushSubscription) (*model.PushSubscription, error)
	GetSubscriptionByID(ctx context.Context, id string) (*model.PushSubscription, error)
	GetSubscriptionByEndpoint(ctx context.Context, endpoint string) (*model.PushSubscription, error)
	ListSubscriptionsByUserID(ctx context.Context, userID string, activeOnly bool) ([]*model.PushSubscription, error)
	UpdateSubscription(ctx context.Context, sub *model.PushSubscription) (*model.PushSubscription, error)
	DeactivateSubscription(ctx context.Context, id, userID, endpoint string) (bool, error)
	MarkSubscriptionGone(ctx context.Context, id string, errMsg string) error
	RecordSubscriptionFailure(ctx context.Context, id string, errMsg string) error
	MarkSubscriptionUsed(ctx context.Context, id string) error
}

type NotificationHistoryRepository interface {
	CreateNotification(ctx context.Context, n *model.Notification) error
	GetNotificationByID(ctx context.Context, id string) (*model.Notification, error)
	GetNotificationByEventIDAndChannel(ctx context.Context, eventID, channelID string) (*model.Notification, error)
	ListNotifications(ctx context.Context, filter model.NotificationFilter) ([]*model.Notification, int, int, error)
	UpdateDeliveryStatus(ctx context.Context, id string, status model.DeliveryStatus, errMsg *string, incrementRetry bool) error
	MarkAsRead(ctx context.Context, id, userID string, isRead bool) (*model.Notification, error)
	MarkAllAsRead(ctx context.Context, userID string) (int, error)
	GetUnreadCount(ctx context.Context, userID string) (int, error)
}

type NotificationRepository interface {
	ChannelRepository
	TemplateRepository
	PreferenceRepository
	JobRepository
	IdempotencyRepository
	PushSubscriptionRepository
	NotificationHistoryRepository
}
