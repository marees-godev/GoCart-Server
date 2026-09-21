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
	Topic         string
	Status        Status
	RetryCount    int
	CreatedAt     time.Time
	PublishedAt   *time.Time
}

// Config controls the background publisher behaviour.
type Config struct {
	// PollInterval is how often the publisher checks for pending events.
	PollInterval time.Duration
	// BatchSize is the maximum number of events claimed per poll cycle.
	BatchSize int
	// MaxRetries is the total number of publish attempts before an event is
	// permanently marked FAILED.
	MaxRetries int
}

// DefaultConfig returns sensible defaults for the outbox publisher.
func DefaultConfig() Config {
	return Config{
		PollInterval: 2 * time.Second,
		BatchSize:    50,
		MaxRetries:   5,
	}
}
