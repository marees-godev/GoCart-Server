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
