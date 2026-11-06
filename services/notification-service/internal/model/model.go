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

type ProcessedEvent struct {
	EventID     string    `json:"event_id"`
	EventType   string    `json:"event_type"`
	Source      string    `json:"source"`
	ProcessedAt time.Time `json:"processed_at"`
}

