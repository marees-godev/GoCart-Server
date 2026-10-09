package outbox

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// Status represents the lifecycle state of an outbox event.
type Status string

const (
	StatusPending   Status = "PENDING"
	StatusPublished Status = "PUBLISHED"
	StatusFailed    Status = "FAILED"
)

// Event is the in-memory representation of a row in outbox_events.
type Event struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte
	Headers       []byte
	Topic         string
	Status        Status
	RetryCount    int
	NextRetryAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	PublishedAt   *time.Time
}

// Config controls the background publisher behaviour.
type Config struct {
	// ServiceName is used for logging and metrics attribution.
	ServiceName string
	// PollInterval is how often the publisher checks for pending events.
	PollInterval time.Duration
	// BatchSize is the maximum number of events claimed per poll cycle.
	BatchSize int
	// MaxRetries is the total number of publish attempts before an event is
	// permanently routed to DLQ and marked FAILED.
	MaxRetries int
	// BaseRetryDelay is the initial delay for exponential backoff retry.
	BaseRetryDelay time.Duration
	// MaxRetryDelay is the upper bound on backoff interval.
	MaxRetryDelay time.Duration
	// DLQTopic specifies the Dead-Letter Queue topic to publish persistently failed events to.
	DLQTopic string
}

// DefaultConfig returns sensible defaults for the outbox publisher.
func DefaultConfig() Config {
	return Config{
		ServiceName:    "gocart-service",
		PollInterval:   2 * time.Second,
		BatchSize:      50,
		MaxRetries:     5,
		BaseRetryDelay: 1 * time.Second,
		MaxRetryDelay:  60 * time.Second,
		DLQTopic:       "",
	}
}
