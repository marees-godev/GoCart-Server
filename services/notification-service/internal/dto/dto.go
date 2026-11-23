package dto

import "github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"

type RegisterPushSubscriptionInput struct {
	UserID     string `json:"user_id"`
	Endpoint   string `json:"endpoint"`
	P256DH     string `json:"p256dh"`
	Auth       string `json:"auth"`
	UserAgent  string `json:"user_agent,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
}

type UpdatePushSubscriptionInput struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	Endpoint   string `json:"endpoint,omitempty"`
	P256DH     string `json:"p256dh,omitempty"`
	Auth       string `json:"auth,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
	IsActive   *bool  `json:"is_active,omitempty"`
}

type DeactivatePushSubscriptionInput struct {
	ID       string `json:"id,omitempty"`
	UserID   string `json:"user_id"`
	Endpoint string `json:"endpoint,omitempty"`
}

type SendNotificationInput struct {
	UserID        string
	Channel       string
	EventType     string
	EventID       string
	Recipient     string
	Subject       string
	Content       string
	TemplateID    string
	TemplateData  map[string]string
	CorrelationID string
}

type SendNotificationResult struct {
	Notification *model.Notification
	Success      bool
	Message      string
}

type ListNotificationsResult struct {
	Notifications []*model.Notification
	Total         int
	UnreadCount   int
}

type PushPayload struct {
	NotificationID string            `json:"notification_id,omitempty"`
	Title          string            `json:"title"`
	Body           string            `json:"body"`
	EventType      string            `json:"event_type,omitempty"`
	CorrelationID  string            `json:"correlation_id,omitempty"`
	Timestamp      string            `json:"timestamp,omitempty"`
	Data           map[string]string `json:"data,omitempty"`
}

