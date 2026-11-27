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
	EventTypeOrderPaymentConfirmed      = "OrderPaymentConfirmed"
	EventTypeOrderShipped               = "OrderShipped"
	EventTypeOrderDelivered             = "OrderDelivered"
	EventTypeOrderPaymentFailed         = "OrderPaymentFailed"
	EventTypePaymentSuccessful          = "PaymentSuccessful"
	EventTypePaymentFailed              = "PaymentFailed"
	EventTypePaymentInitiated           = "PaymentInitiated"
	EventTypePaymentPending             = "PaymentPending"
	EventTypePaymentCancelled           = "PaymentCancelled"
	EventTypeRefundInitiated            = "RefundInitiated"
	EventTypeRefundCompleted            = "RefundCompleted"
	EventTypeRefundFailed               = "RefundFailed"
	EventTypeInventoryReserved          = "InventoryReserved"
	EventTypeInventoryReleased          = "InventoryReleased"
	EventTypeInventoryCommitted         = "InventoryCommitted"
	EventTypeInventoryLow               = "InventoryLow"
	EventTypeInventoryCreated           = "InventoryCreated"
	EventTypeInventoryRestocked         = "InventoryRestocked"
	EventTypeInventoryReservationFailed = "InventoryReservationFailed"
	EventTypeDeliveryDispatched         = "DeliveryDispatched"
	EventTypeUserRegistered             = "UserRegistered"
	EventTypePartnerRegistered          = "PartnerRegistered"
	EventTypeUserLoggedIn               = "UserLoggedIn"
	EventTypePasswordReset              = "PasswordReset"
	EventTypePasswordChanged            = "PasswordChanged"

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
	EventTypeMerchantApproved   = "MerchantApproved"
	EventTypeMerchantRejected   = "MerchantRejected"
	EventTypeMerchantActivated  = "MerchantActivated"
	EventTypeMerchantSuspended  = "MerchantSuspended"

	// Delivery Service Events
	EventTypeDeliveryCreated       = "DeliveryCreated"
	EventTypeDeliveryStatusUpdated = "DeliveryStatusUpdated"
	EventTypeDeliveryStatusChanged = "DeliveryStatusChanged"

	// Product Service Events
	EventTypeProductCreated   = "ProductCreated"
	EventTypeProductUpdated   = "ProductUpdated"
	EventTypeProductPublished = "ProductPublished"
	EventTypeProductDeleted   = "ProductDeleted"
	EventTypeProductReported  = "ProductReported"

	// Return Service Events
	EventTypeReturnRequested      = "ReturnRequested"
	EventTypeReturnApproved       = "ReturnApproved"
	EventTypeReturnRejected       = "ReturnRejected"
	EventTypeReturnVerified       = "ReturnVerified"
	EventTypeReturnRefundEligible = "ReturnRefundEligible"
	EventTypeReturnStatusUpdated  = "ReturnStatusUpdated"
	EventTypeReturnCompleted      = "ReturnCompleted"
	EventTypeExchangeCreated      = "ExchangeCreated"
	EventTypeExchangeOfferApplied = "ExchangeOfferApplied"
	EventTypeExchangeVerified     = "ExchangeVerified"
	EventTypeExchangeCompleted    = "ExchangeCompleted"

	// Personalization Service Events
	EventTypeProductViewed          = "ProductViewed"
	EventTypeProductSearched        = "ProductSearched"
	EventTypeCategoryViewed         = "CategoryViewed"
	EventTypeProductAddedToCart     = "ProductAddedToCart"
	EventTypeProductRemovedFromCart = "ProductRemovedFromCart"
	EventTypeProductWishlisted      = "ProductWishlisted"
	EventTypeProductPurchased       = "ProductPurchased"
)

// Standard Topic Constants (Consolidated 5 Domain Topics)
const (
	TopicUserRegistered        = "gocart.accounts"
	TopicPartnerRegistered     = "gocart.accounts"
	TopicUserLoggedIn          = "gocart.accounts"
	TopicPasswordReset         = "gocart.accounts"
	TopicPasswordChanged       = "gocart.accounts"
	TopicOrderCreated          = "gocart.orders"
	TopicOrderCancelled        = "gocart.orders"
	TopicOrderConfirmed        = "gocart.orders"
	TopicOrderPaymentConfirmed = "gocart.orders"
	TopicOrderShipped          = "gocart.orders"
	TopicOrderDelivered        = "gocart.orders"
	TopicOrderPaymentFailed    = "gocart.orders"
	TopicPaymentSuccessful     = "gocart.payments"
	TopicPaymentFailed         = "gocart.payments"
	TopicPaymentInitiated      = "gocart.payments"
	TopicPaymentPending        = "gocart.payments"
	TopicPaymentCancelled      = "gocart.payments"
	TopicRefundInitiated       = "gocart.payments"
	TopicRefundCompleted       = "gocart.payments"
	TopicRefundFailed          = "gocart.payments"
	TopicInventoryReserved     = "gocart.orders"
	TopicInventoryReleased     = "gocart.orders"
	TopicInventoryCommitted    = "gocart.orders"
	TopicInventoryLow          = "gocart.orders"
	TopicInventoryCreated      = "gocart.orders"
	TopicInventoryRestocked    = "gocart.orders"

	// Store Service Topics
	TopicStoreCreated     = "gocart.accounts"
	TopicStoreSubmitted   = "gocart.accounts"
	TopicStoreApproved    = "gocart.accounts"
	TopicStoreRejected    = "gocart.accounts"
	TopicStoreSuspended   = "gocart.accounts"
	TopicStoreUnsuspended = "gocart.accounts"
	TopicStoreAppealed    = "gocart.accounts"

	// Merchant Service Topics
	TopicMerchantRegistered = "gocart.accounts"
	TopicMerchantApproved   = "gocart.accounts"
	TopicMerchantRejected   = "gocart.accounts"
	TopicMerchantActivated  = "gocart.accounts"
	TopicMerchantSuspended  = "gocart.accounts"

	// Delivery Service Topics
	TopicDeliveryCreated       = "gocart.deliveries"
	TopicDeliveryDispatched    = "gocart.deliveries"
	TopicDeliveryStatusUpdated = "gocart.deliveries"
	TopicDeliveryStatusChanged = "gocart.deliveries"

	// Product Service Topics
	TopicProductCreated   = "gocart.product.product-created"
	TopicProductUpdated   = "gocart.product.product-updated"
	TopicProductPublished = "gocart.product.product-published"
	TopicProductDeleted   = "gocart.product.product-deleted"
	TopicProductReported  = "gocart.product.product-reported"

	// Return Service Topics
	TopicReturnRequested      = "gocart.orders"
	TopicReturnApproved       = "gocart.orders"
	TopicReturnRejected       = "gocart.orders"
	TopicReturnVerified       = "gocart.orders"
	TopicReturnRefundEligible = "gocart.orders"
	TopicReturnStatusUpdated  = "gocart.orders"
	TopicReturnCompleted      = "gocart.orders"
	TopicExchangeCreated      = "gocart.orders"
	TopicExchangeOfferApplied = "gocart.orders"
	TopicExchangeVerified     = "gocart.orders"
	TopicExchangeCompleted    = "gocart.orders"

	// Personalization Service Topics
	TopicCustomerActivity = "gocart.accounts"

	// Unified Dead Letter Queue (DLQ) Topic
	TopicDLQ = "gocart.dlq"
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
