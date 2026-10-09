package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
)

// Standard Event Type Constants
const (
	EventTypeOrderCreated               = "OrderCreated"
	EventTypeOrderCancelled             = "OrderCancelled"
	EventTypeOrderConfirmed             = "OrderConfirmed"
	EventTypePaymentSuccessful          = "PaymentSuccessful"
	EventTypePaymentFailed              = "PaymentFailed"
	EventTypeInventoryReserved          = "InventoryReserved"
	EventTypeInventoryReleased          = "InventoryReleased"
	EventTypeInventoryCommitted         = "InventoryCommitted"
	EventTypeInventoryLow               = "InventoryLow"
	EventTypeInventoryCreated           = "InventoryCreated"
	EventTypeInventoryRestocked         = "InventoryRestocked"
	EventTypeInventoryReservationFailed = "InventoryReservationFailed"
	EventTypeDeliveryDispatched         = "DeliveryDispatched"
	EventTypeUserRegistered             = "UserRegistered"

	// Store Service Events
	EventTypeStoreCreated     = "StoreCreated"
	EventTypeStoreSubmitted   = "StoreSubmitted"
	EventTypeStoreApproved    = "StoreApproved"
	EventTypeStoreRejected    = "StoreRejected"
	EventTypeStoreSuspended   = "StoreSuspended"
	EventTypeStoreUnsuspended = "StoreUnsuspended"
	EventTypeStoreAppealed    = "StoreAppealed"

	// Merchant Service Events
	EventTypeMerchantRegistered = "MerchantRegistered"
	EventTypeMerchantActivated  = "MerchantActivated"
	EventTypeMerchantSuspended  = "MerchantSuspended"
)

// Standard Topic Constants
const (
	TopicUserRegistered     = "gocart.auth.user-registered"
	TopicOrderCreated       = "gocart.order.order-created"
	TopicOrderCancelled     = "gocart.order.order-cancelled"
	TopicOrderConfirmed     = "gocart.order.order-confirmed"
	TopicPaymentFailed      = "gocart.payment.payment-failed"
	TopicInventoryReserved  = "gocart.inventory.inventory-reserved"
	TopicInventoryReleased  = "gocart.inventory.inventory-released"
	TopicInventoryCommitted = "gocart.inventory.inventory-committed"
	TopicInventoryLow       = "gocart.inventory.inventory-low"
	TopicInventoryCreated   = "gocart.inventory.inventory-created"
	TopicInventoryRestocked = "gocart.inventory.inventory-restocked"

	// Store Service Topics
	TopicStoreCreated     = "gocart.store.store-created"
	TopicStoreSubmitted   = "gocart.store.store-submitted"
	TopicStoreApproved    = "gocart.store.store-approved"
	TopicStoreRejected    = "gocart.store.store-rejected"
	TopicStoreSuspended   = "gocart.store.store-suspended"
	TopicStoreUnsuspended = "gocart.store.store-unsuspended"
	TopicStoreAppealed    = "gocart.store.store-appealed"

	// Merchant Service Topics
	TopicMerchantRegistered = "gocart.merchant.merchant-registered"
	TopicMerchantActivated  = "gocart.merchant.merchant-activated"
	TopicMerchantSuspended  = "gocart.merchant.merchant-suspended"
)

// EventEnvelope is the standard CloudEvents-compliant envelope for all domain events across GoCart.
type EventEnvelope struct {
	EventID       string            `json:"event_id"`
	EventType     string            `json:"event_type"`
	AggregateID   string            `json:"aggregate_id,omitempty"`
	EventVersion  string            `json:"event_version"`
	Source        string            `json:"source"`
	Timestamp     time.Time         `json:"timestamp"`
	OccurredAt    time.Time         `json:"occurred_at,omitempty"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	TraceID       string            `json:"trace_id,omitempty"`
	Data          json.RawMessage   `json:"data"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// NewEventEnvelope creates a new standard EventEnvelope for a given payload.
func NewEventEnvelope(eventType, source string, payload interface{}) (*EventEnvelope, error) {
	return NewEventEnvelopeWithAggregate(eventType, source, "", payload)
}

// NewEventEnvelopeWithAggregate creates a new standard EventEnvelope including aggregate ID.
func NewEventEnvelopeWithAggregate(eventType, source, aggregateID string, payload interface{}) (*EventEnvelope, error) {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event payload: %w", err)
	}

	now := time.Now().UTC()
	return &EventEnvelope{
		EventID:      uuid.Must(uuid.NewV7()).String(),
		EventType:    eventType,
		AggregateID:  aggregateID,
		EventVersion: "1.0",
		Source:       source,
		Timestamp:    now,
		OccurredAt:   now,
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
