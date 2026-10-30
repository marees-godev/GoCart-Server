package events_test

import (
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
)

func TestEventEnvelopeSerialization(t *testing.T) {
	payload := events.OrderCreatedEvent{
		OrderID:     "ord-12345",
		UserID:      "usr-67890",
		TotalAmount: 99.99,
		Currency:    "USD",
		Status:      "PENDING",
		CreatedAt:   time.Now().UTC(),
		Items: []events.OrderItemPayload{
			{ProductID: "prod-1", Quantity: 2, UnitPrice: 49.995},
		},
	}

	env, err := events.NewEventEnvelope(events.EventTypeOrderCreated, "order-service", payload)
	if err != nil {
		t.Fatalf("failed to create event envelope: %v", err)
	}

	if env.EventID == "" {
		t.Error("expected non-empty EventID")
	}
	if env.EventType != events.EventTypeOrderCreated {
		t.Errorf("expected EventType %s, got %s", events.EventTypeOrderCreated, env.EventType)
	}
	if env.Source != "order-service" {
		t.Errorf("expected Source order-service, got %s", env.Source)
	}

	bytes, err := env.Marshal()
	if err != nil {
		t.Fatalf("failed to marshal envelope: %v", err)
	}

	unmarshaledEnv, err := events.UnmarshalEnvelope(bytes)
	if err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}

	var restoredPayload events.OrderCreatedEvent
	if err := unmarshaledEnv.UnmarshalData(&restoredPayload); err != nil {
		t.Fatalf("failed to unmarshal payload from envelope: %v", err)
	}

	if restoredPayload.OrderID != payload.OrderID {
		t.Errorf("expected OrderID %s, got %s", payload.OrderID, restoredPayload.OrderID)
	}
	if len(restoredPayload.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(restoredPayload.Items))
	}
	if restoredPayload.Items[0].ProductID != "prod-1" {
		t.Errorf("expected ProductID prod-1, got %s", restoredPayload.Items[0].ProductID)
	}
}

func TestUserRegisteredEventSerialization(t *testing.T) {
	now := time.Now().UTC()
	payload := events.UserRegisteredEvent{
		UserID:       "usr-merch-12345",
		Email:        "merchant@example.com",
		Role:         "MERCHANT",
		BusinessName: "Acme Supplies",
		Phone:        "+1234567890",
		CreatedAt:    now,
	}

	env, err := events.NewEventEnvelope(events.EventTypeUserRegistered, "auth-service", payload)
	if err != nil {
		t.Fatalf("failed to create event envelope: %v", err)
	}

	if env.EventType != events.EventTypeUserRegistered {
		t.Errorf("expected EventType %s, got %s", events.EventTypeUserRegistered, env.EventType)
	}

	bytes, err := env.Marshal()
	if err != nil {
		t.Fatalf("failed to marshal envelope: %v", err)
	}

	unmarshaledEnv, err := events.UnmarshalEnvelope(bytes)
	if err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}

	var restored events.UserRegisteredEvent
	if err := unmarshaledEnv.UnmarshalData(&restored); err != nil {
		t.Fatalf("failed to unmarshal UserRegisteredEvent: %v", err)
	}

	if restored.UserID != payload.UserID {
		t.Errorf("expected UserID %s, got %s", payload.UserID, restored.UserID)
	}
	if restored.Email != payload.Email {
		t.Errorf("expected Email %s, got %s", payload.Email, restored.Email)
	}
	if restored.Role != payload.Role {
		t.Errorf("expected Role %s, got %s", payload.Role, restored.Role)
	}
	if restored.BusinessName != payload.BusinessName {
		t.Errorf("expected BusinessName %s, got %s", payload.BusinessName, restored.BusinessName)
	}
}

func TestStoreEventsSerialization(t *testing.T) {
	now := time.Now().UTC()

	t.Run("StoreCreatedEvent", func(t *testing.T) {
		payload := events.StoreCreatedEvent{
			StoreID:       "st-1",
			MerchantID:    "m-1",
			Name:          "Tech Store",
			Slug:          "tech-store",
			BusinessEmail: "tech@example.com",
			BusinessPhone: "+1234567890",
			CreatedAt:     now,
		}
		env, err := events.NewEventEnvelope(events.EventTypeStoreCreated, "store-service", payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.StoreCreatedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.StoreID != "st-1" || restored.MerchantID != "m-1" || restored.Slug != "tech-store" {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("StoreSubmittedEvent", func(t *testing.T) {
		payload := events.StoreSubmittedEvent{
			StoreID:     "st-1",
			MerchantID:  "m-1",
			SubmittedAt: now,
		}
		env, err := events.NewEventEnvelope(events.EventTypeStoreSubmitted, "store-service", payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.StoreSubmittedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.StoreID != "st-1" || restored.MerchantID != "m-1" {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("StoreApprovedEvent", func(t *testing.T) {
		payload := events.StoreApprovedEvent{
			StoreID:    "st-1",
			MerchantID: "m-1",
			AdminID:    "admin-1",
			ApprovedAt: now,
		}
		env, err := events.NewEventEnvelope(events.EventTypeStoreApproved, "store-service", payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.StoreApprovedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.StoreID != "st-1" || restored.AdminID != "admin-1" {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("StoreRejectedEvent", func(t *testing.T) {
		payload := events.StoreRejectedEvent{
			StoreID:    "st-1",
			MerchantID: "m-1",
			AdminID:    "admin-1",
			Reason:     "Invalid docs",
			RejectedAt: now,
		}
		env, err := events.NewEventEnvelope(events.EventTypeStoreRejected, "store-service", payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.StoreRejectedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.StoreID != "st-1" || restored.Reason != "Invalid docs" {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("StoreSuspendedEvent", func(t *testing.T) {
		payload := events.StoreSuspendedEvent{
			StoreID:     "st-1",
			MerchantID:  "m-1",
			AdminID:     "admin-1",
			Reason:      "Policy violation",
			SuspendedAt: now,
		}
		env, err := events.NewEventEnvelope(events.EventTypeStoreSuspended, "store-service", payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.StoreSuspendedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.StoreID != "st-1" || restored.Reason != "Policy violation" {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("MerchantRegisteredEvent", func(t *testing.T) {
		payload := events.MerchantRegisteredEvent{
			MerchantID:   "merch-123",
			Email:        "merchant@example.com",
			BusinessName: "Acme Corp",
			FirstName:    "John",
			LastName:     "Doe",
			Phone:        "+1234567890",
			CreatedAt:    now,
			RegisteredAt: now,
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeMerchantRegistered, "merchant-service", payload.MerchantID, payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		if env.AggregateID != "merch-123" {
			t.Errorf("expected aggregate_id merch-123, got %s", env.AggregateID)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.MerchantRegisteredEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.MerchantID != "merch-123" || restored.BusinessName != "Acme Corp" {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("MerchantActivatedEvent", func(t *testing.T) {
		payload := events.MerchantActivatedEvent{
			MerchantID:     "merch-123",
			PreviousStatus: "PENDING",
			NewStatus:      "ACTIVE",
			ActivatedBy:    "admin-uuid-1",
			Reason:         "KYC verified",
			ActivatedAt:    now,
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeMerchantActivated, "merchant-service", payload.MerchantID, payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.MerchantActivatedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.MerchantID != "merch-123" || restored.NewStatus != "ACTIVE" || restored.ActivatedBy != "admin-uuid-1" {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("MerchantSuspendedEvent", func(t *testing.T) {
		payload := events.MerchantSuspendedEvent{
			MerchantID:     "merch-123",
			PreviousStatus: "ACTIVE",
			NewStatus:      "SUSPENDED",
			SuspendedBy:    "admin-uuid-1",
			Reason:         "Terms violation",
			SuspendedAt:    now,
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeMerchantSuspended, "merchant-service", payload.MerchantID, payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.MerchantSuspendedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.MerchantID != "merch-123" || restored.NewStatus != "SUSPENDED" || restored.Reason != "Terms violation" {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("PaymentCancelledEvent", func(t *testing.T) {
		payload := events.PaymentCancelledEvent{
			PaymentID:   "pay-cancelled-1",
			OrderID:     "ord-cancelled-1",
			UserID:      "usr-1",
			Amount:      100.0,
			Currency:    "USD",
			Reason:      "User requested cancellation",
			CancelledAt: now,
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypePaymentCancelled, "payment-service", payload.PaymentID, payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.PaymentCancelledEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.PaymentID != "pay-cancelled-1" || restored.Reason != "User requested cancellation" {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("RefundInitiatedEvent", func(t *testing.T) {
		payload := events.RefundInitiatedEvent{
			RefundID:    "ref-init-1",
			PaymentID:   "pay-ref-1",
			OrderID:     "ord-ref-1",
			Amount:      45.50,
			Currency:    "USD",
			Reason:      "Return approved",
			InitiatedAt: now,
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeRefundInitiated, "payment-service", payload.RefundID, payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.RefundInitiatedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.RefundID != "ref-init-1" || restored.Amount != 45.50 {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("RefundCompletedEvent", func(t *testing.T) {
		payload := events.RefundCompletedEvent{
			RefundID:        "ref-comp-1",
			PaymentID:       "pay-ref-1",
			OrderID:         "ord-ref-1",
			Amount:          45.50,
			Currency:        "USD",
			GatewayRefundID: "gw_ref_123",
			IsPartial:       true,
			PaymentStatus:   "PARTIALLY_REFUNDED",
			CompletedAt:     now,
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeRefundCompleted, "payment-service", payload.RefundID, payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.RefundCompletedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.RefundID != "ref-comp-1" || !restored.IsPartial || restored.GatewayRefundID != "gw_ref_123" {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("RefundFailedEvent", func(t *testing.T) {
		payload := events.RefundFailedEvent{
			RefundID:  "ref-fail-1",
			PaymentID: "pay-ref-1",
			OrderID:   "ord-ref-1",
			Amount:    45.50,
			Reason:    "Gateway timeout",
			FailedAt:  now,
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeRefundFailed, "payment-service", payload.RefundID, payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.RefundFailedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.RefundID != "ref-fail-1" || restored.Reason != "Gateway timeout" {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})
	t.Run("ProductCreatedEvent", func(t *testing.T) {
		payload := events.ProductCreatedEvent{
			ProductID:   "prod-101",
			StoreID:     "store-202",
			CategoryID:  "cat-303",
			SKU:         "SKU-ABC-1",
			Name:        "Wireless Mechanical Keyboard",
			Description: "High-performance wireless keyboard",
			Price:       89.99,
			Currency:    "USD",
			Status:      "DRAFT",
			CreatedAt:   now,
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeProductCreated, "product-service", payload.ProductID, payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		if env.EventType != events.EventTypeProductCreated {
			t.Errorf("expected EventType %s, got %s", events.EventTypeProductCreated, env.EventType)
		}
		if env.AggregateID != "prod-101" {
			t.Errorf("expected aggregate_id prod-101, got %s", env.AggregateID)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.ProductCreatedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.ProductID != "prod-101" || restored.StoreID != "store-202" || restored.Price != 89.99 {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("ProductUpdatedEvent", func(t *testing.T) {
		payload := events.ProductUpdatedEvent{
			ProductID:   "prod-101",
			StoreID:     "store-202",
			CategoryID:  "cat-303",
			SKU:         "SKU-ABC-1",
			Name:        "Wireless Mechanical Keyboard Pro",
			Description: "Updated high-performance wireless keyboard",
			Price:       99.99,
			Currency:    "USD",
			Status:      "DRAFT",
			UpdatedAt:   now,
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeProductUpdated, "product-service", payload.ProductID, payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.ProductUpdatedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.ProductID != "prod-101" || restored.StoreID != "store-202" || restored.Price != 99.99 {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("ProductPublishedEvent", func(t *testing.T) {
		payload := events.ProductPublishedEvent{
			ProductID:   "prod-101",
			StoreID:     "store-202",
			CategoryID:  "cat-303",
			SKU:         "SKU-ABC-1",
			Name:        "Wireless Mechanical Keyboard Pro",
			Price:       99.99,
			Currency:    "USD",
			PublishedAt: now,
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeProductPublished, "product-service", payload.ProductID, payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.ProductPublishedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.ProductID != "prod-101" || restored.StoreID != "store-202" || restored.Price != 99.99 {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("ProductDeletedEvent", func(t *testing.T) {
		payload := events.ProductDeletedEvent{
			ProductID: "prod-101",
			StoreID:   "store-202",
			Status:    "DELETED",
			DeletedAt: now,
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeProductDeleted, "product-service", payload.ProductID, payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.ProductDeletedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.ProductID != "prod-101" || restored.StoreID != "store-202" || restored.Status != "DELETED" {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})

	t.Run("OrderPaymentFailedEvent", func(t *testing.T) {
		now := time.Now().UTC()
		payload := events.OrderPaymentFailedEvent{
			OrderID:   "ord-fail-999",
			PaymentID: "pay-fail-888",
			Reason:    "Insufficient funds",
			FailedAt:  now,
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderPaymentFailed, "order-service", payload.OrderID, payload)
		if err != nil {
			t.Fatalf("failed creating envelope: %v", err)
		}
		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling envelope: %v", err)
		}
		unmarshaled, err := events.UnmarshalEnvelope(bytes)
		if err != nil {
			t.Fatalf("failed unmarshaling envelope: %v", err)
		}
		var restored events.OrderPaymentFailedEvent
		if err := unmarshaled.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed unmarshaling data: %v", err)
		}
		if restored.OrderID != "ord-fail-999" || restored.PaymentID != "pay-fail-888" || restored.Reason != "Insufficient funds" {
			t.Errorf("restored payload mismatch: %+v", restored)
		}
	})
}

