package outbox

import (
	"context"

	"github.com/marees-godev/GoCart-Server/contracts/events"
)

// KafkaPublisher is the narrow interface the outbox publisher requires from the
// Kafka layer. Using an interface keeps pkg/outbox free of a hard dependency on
// pkg/kafka and makes the publisher unit-testable.
type KafkaPublisher interface {
	Publish(ctx context.Context, topic, key string, envelope *events.EventEnvelope) error
}
