package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Standard Event Type Constants
const (
	EventTypeOrderCreated               = "OrderCreated"
	EventTypePaymentSuccessful          = "PaymentSuccessful"
	EventTypePaymentFailed              = "PaymentFailed"
	EventTypeInventoryReserved          = "InventoryReserved"
	EventTypeInventoryReservationFailed = "InventoryReservationFailed"
	EventTypeDeliveryDispatched         = "DeliveryDispatched"
)

// EventEnvelope is the standard envelope for all domain events across GoCart.
type EventEnvelope struct {
	EventID       string            `json:"event_id"`
	EventType     string            `json:"event_type"`
	EventVersion  string            `json:"event_version"`
	Source        string            `json:"source"`
	Timestamp     time.Time         `json:"timestamp"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	TraceID       string            `json:"trace_id,omitempty"`
	Data          json.RawMessage   `json:"data"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// NewEventEnvelope creates a new standard EventEnvelope for a given payload.
func NewEventEnvelope(eventType, source string, payload interface{}) (*EventEnvelope, error) {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event payload: %w", err)
	}

	return &EventEnvelope{
		EventID:      uuid.New().String(),
		EventType:    eventType,
		EventVersion: "1.0",
		Source:       source,
		Timestamp:    time.Now().UTC(),
		Data:         dataBytes,
		Metadata:     make(map[string]string),
	}, nil
}

// UnmarshalData unmarshals the raw JSON data of the envelope into a target domain event struct.
func (e *EventEnvelope) UnmarshalData(target interface{}) error {
	if len(e.Data) == 0 {
		return fmt.Errorf("envelope data is empty")
	}
	if err := json.Unmarshal(e.Data, target); err != nil {
		return fmt.Errorf("failed to unmarshal event data into target: %w", err)
	}
	return nil
}

// Marshal serializes the EventEnvelope to JSON.
func (e *EventEnvelope) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

// UnmarshalEnvelope deserializes a JSON payload into an EventEnvelope.
func UnmarshalEnvelope(data []byte) (*EventEnvelope, error) {
	var env EventEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event envelope: %w", err)
	}
	return &env, nil
}
