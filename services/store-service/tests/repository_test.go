package tests_test

import (
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/model"
)

type mockOutboxStore struct {
	events []*outbox.Event
}

func newMockOutboxStore() *mockOutboxStore {
	return &mockOutboxStore{
		events: make([]*outbox.Event, 0),
	}
}

func (m *mockOutboxStore) Insert(evt *outbox.Event) {
	m.events = append(m.events, evt)
}

func TestRepositoryOutboxEvents(t *testing.T) {
	now := time.Now().UTC()

	t.Run("StoreCreated Outbox Envelope Structure", func(t *testing.T) {
		payload := events.StoreCreatedEvent{
			StoreID:       "store-uuid-1",
			MerchantID:    "merchant-uuid-1",
			Name:          "SuperMart",
			Slug:          "supermart",
			BusinessEmail: "super@example.com",
			BusinessPhone: "+15550199",
			CreatedAt:     now,
		}

		env, err := events.NewEventEnvelope(events.EventTypeStoreCreated, "store-service", payload)
		if err != nil {
			t.Fatalf("failed to create envelope: %v", err)
		}

		if env.EventType != events.EventTypeStoreCreated {
			t.Errorf("expected EventType %s, got %s", events.EventTypeStoreCreated, env.EventType)
		}
		if env.Source != "store-service" {
			t.Errorf("expected Source store-service, got %s", env.Source)
		}

		bytes, err := env.Marshal()
		if err != nil {
			t.Fatalf("failed to marshal envelope: %v", err)
		}

		evt := &outbox.Event{
			AggregateType: "store",
			AggregateID:   payload.StoreID,
			EventType:     env.EventType,
			Payload:       bytes,
			Topic:         events.TopicStoreCreated,
		}

		if evt.Topic != "gocart.store.store-created" {
			t.Errorf("expected topic gocart.store.store-created, got %s", evt.Topic)
		}
		if evt.AggregateID != "store-uuid-1" {
			t.Errorf("expected aggregate_id store-uuid-1, got %s", evt.AggregateID)
		}

		// Verify unmarshal
		unmarshaledEnv, err := events.UnmarshalEnvelope(evt.Payload)
		if err != nil {
			t.Fatalf("failed to unmarshal envelope from payload: %v", err)
		}
		var restored events.StoreCreatedEvent
		if err := unmarshaledEnv.UnmarshalData(&restored); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}
		if restored.StoreID != "store-uuid-1" || restored.MerchantID != "merchant-uuid-1" || restored.Name != "SuperMart" {
			t.Errorf("restored event payload mismatch: %+v", restored)
		}
	})

	t.Run("StoreSubmitted Outbox Event", func(t *testing.T) {
		payload := events.StoreSubmittedEvent{
			StoreID:     "store-uuid-1",
			MerchantID:  "merchant-uuid-1",
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

		evt := &outbox.Event{
			AggregateType: "store",
			AggregateID:   payload.StoreID,
			EventType:     events.EventTypeStoreSubmitted,
			Payload:       bytes,
			Topic:         events.TopicStoreSubmitted,
		}

		if evt.Topic != "gocart.store.store-submitted" {
			t.Errorf("expected topic gocart.store.store-submitted, got %s", evt.Topic)
		}
	})

	t.Run("StoreApproved Outbox Event", func(t *testing.T) {
		payload := events.StoreApprovedEvent{
			StoreID:    "store-uuid-1",
			MerchantID: "merchant-uuid-1",
			AdminID:    "admin-uuid-1",
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

		evt := &outbox.Event{
			AggregateType: "store",
			AggregateID:   payload.StoreID,
			EventType:     events.EventTypeStoreApproved,
			Payload:       bytes,
			Topic:         events.TopicStoreApproved,
		}

		if evt.Topic != "gocart.store.store-approved" {
			t.Errorf("expected topic gocart.store.store-approved, got %s", evt.Topic)
		}
	})

	t.Run("StoreRejected Outbox Event", func(t *testing.T) {
		payload := events.StoreRejectedEvent{
			StoreID:    "store-uuid-1",
			MerchantID: "merchant-uuid-1",
			AdminID:    "admin-uuid-1",
			Reason:     "Blurry identity proof",
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

		evt := &outbox.Event{
			AggregateType: "store",
			AggregateID:   payload.StoreID,
			EventType:     events.EventTypeStoreRejected,
			Payload:       bytes,
			Topic:         events.TopicStoreRejected,
		}

		if evt.Topic != "gocart.store.store-rejected" {
			t.Errorf("expected topic gocart.store.store-rejected, got %s", evt.Topic)
		}
	})

	t.Run("StoreSuspended Outbox Event", func(t *testing.T) {
		payload := events.StoreSuspendedEvent{
			StoreID:     "store-uuid-1",
			MerchantID:  "merchant-uuid-1",
			AdminID:     "admin-uuid-1",
			Reason:      "Fraudulent activity",
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

		evt := &outbox.Event{
			AggregateType: "store",
			AggregateID:   payload.StoreID,
			EventType:     events.EventTypeStoreSuspended,
			Payload:       bytes,
			Topic:         events.TopicStoreSuspended,
		}

		if evt.Topic != "gocart.store.store-suspended" {
			t.Errorf("expected topic gocart.store.store-suspended, got %s", evt.Topic)
		}
	})
}

func TestStoreModelConstraintsAndDefaults(t *testing.T) {
	store := model.Store{
		ID:         "s-1",
		MerchantID: "m-1",
		Name:       "Test Store",
		Slug:       "test-store",
	}

	if store.ApprovalStatus != "" {
		t.Errorf("default empty approval status before repo insert")
	}

	if model.StoreStatusDraft != "DRAFT" {
		t.Errorf("expected StoreStatusDraft to be DRAFT")
	}
	if model.StoreStatusPendingApproval != "PENDING_APPROVAL" {
		t.Errorf("expected StoreStatusPendingApproval to be PENDING_APPROVAL")
	}
	if model.StoreStatusApproved != "APPROVED" {
		t.Errorf("expected StoreStatusApproved to be APPROVED")
	}
	if model.StoreStatusRejected != "REJECTED" {
		t.Errorf("expected StoreStatusRejected to be REJECTED")
	}
	if model.StoreStatusSuspended != "SUSPENDED" {
		t.Errorf("expected StoreStatusSuspended to be SUSPENDED")
	}
	if model.StoreStatusClosed != "CLOSED" {
		t.Errorf("expected StoreStatusClosed to be CLOSED")
	}
}
