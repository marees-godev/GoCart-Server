package model

import (
	"time"
)

type JobStatus string

const (
	JobStatusPending    JobStatus = "PENDING"
	JobStatusProcessing JobStatus = "PROCESSING"
	JobStatusCompleted  JobStatus = "COMPLETED"
	JobStatusFailed     JobStatus = "FAILED"
	JobStatusOptedOut   JobStatus = "OPTED_OUT"
)

type DeliveryStatus string

const (
	DeliveryStatusPending DeliveryStatus = "PENDING"
	DeliveryStatusSent    DeliveryStatus = "SENT"
	DeliveryStatusFailed  DeliveryStatus = "FAILED"
)

const (
	ChannelEmail = "EMAIL"
	ChannelSMS   = "SMS"
	ChannelInApp = "IN_APP"
	ChannelPush  = "PUSH"
)

type Channel struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

type Template struct {
	ID              string    `json:"id"`
	EventType       string    `json:"event_type"`
	ChannelID       string    `json:"channel_id"`
	SubjectTemplate string    `json:"subject_template"`
	BodyTemplate    string    `json:"body_template"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Preference struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ChannelID string    `json:"channel_id"`
	EventType string    `json:"event_type"`
	IsEnabled bool      `json:"is_enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type NotificationJob struct {
	ID            string     `json:"id"`
	EventID       string     `json:"event_id"`
	UserID        *string    `json:"user_id,omitempty"`
	ChannelID     string     `json:"channel_id"`
	ChannelCode   string     `json:"channel_code,omitempty"`
	TemplateID    string     `json:"template_id"`
	EventType     string     `json:"event_type"`
	Recipient     string     `json:"recipient"`
	Subject       string     `json:"subject"`
	Content       string     `json:"content"`
	Status        JobStatus  `json:"status"`
	RetryCount    int        `json:"retry_count"`
	MaxRetries    int        `json:"max_retries"`
	CorrelationID string     `json:"correlation_id,omitempty"`
	ErrorMessage  *string    `json:"error_message,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ProcessedAt   *time.Time `json:"processed_at,omitempty"`
}

type Notification struct {
	ID            string         `json:"id"`
	UserID        string         `json:"user_id"`
	ChannelID     string         `json:"channel_id"`
	ChannelCode   string         `json:"channel_code"`
	EventType     string         `json:"event_type"`
	EventID       *string        `json:"event_id,omitempty"`
	Recipient     string         `json:"recipient"`
	Title         string         `json:"title"`
	Content       string         `json:"content"`
	Status        DeliveryStatus `json:"status"`
	ErrorMessage  *string        `json:"error_message,omitempty"`
	IsRead        bool           `json:"is_read"`
	ReadAt        *time.Time     `json:"read_at,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	RetryCount    int            `json:"retry_count"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	SentAt        *time.Time     `json:"sent_at,omitempty"`
}

type NotificationFilter struct {
	UserID    string
	Status    string
	Channel   string
	EventType string
	IsRead    *bool
	Limit     int
	Offset    int
}

type PushSubscription struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	Endpoint      string     `json:"endpoint"`
	P256DH        string     `json:"p256dh"`
	Auth          string     `json:"auth"`
	UserAgent     string     `json:"user_agent,omitempty"`
	DeviceName    string     `json:"device_name,omitempty"`
	IsActive      bool       `json:"is_active"`
	FailureCount  int        `json:"failure_count"`
	LastError     *string    `json:"last_error,omitempty"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	DeactivatedAt *time.Time `json:"deactivated_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type ProcessedEvent struct {
	EventID     string    `json:"event_id"`
	EventType   string    `json:"event_type"`
	Source      string    `json:"source"`
	ProcessedAt time.Time `json:"processed_at"`
}
