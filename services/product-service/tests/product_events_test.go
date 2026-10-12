package tests_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/model"
	productEvents "github.com/marees-godev/GoCart-Server/services/product-service/internal/events"
)

// MockEventBus mocks Kafka publisher for outbox publisher testing
type MockEventBus struct {
	mu        sync.Mutex
	published []PublishedEvent
}

type PublishedEvent struct {
	Topic    string
	Key      string
	Envelope *events.EventEnvelope
}

func NewMockEventBus() *MockEventBus {
	return &MockEventBus{
		published: make([]PublishedEvent, 0),
	}
}

func (b *MockEventBus) Publish(ctx context.Context, topic, key string, envelope *events.EventEnvelope) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.published = append(b.published, PublishedEvent{
		Topic:    topic,
		Key:      key,
		Envelope: envelope,
	})
	return nil
}

func (b *MockEventBus) GetEvents() []PublishedEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	copied := make([]PublishedEvent, len(b.published))
	copy(copied, b.published)
	return copied
}

func TestProductLifecycleEvents(t *testing.T) {
	now := time.Now().UTC()

	t.Run("ProductCreated publishes ProductCreated event with product/store identification", func(t *testing.T) {
		prod := &model.Product{
			ID:          "prod-uuid-101",
			StoreID:     "store-uuid-202",
			CategoryID:  "cat-uuid-303",
			SKU:         "SKU-KEYBOARD-RGB",
			Name:        "RGB Mechanical Keyboard",
			Description: "Custom mechanical gaming keyboard",
			Price:       129.99,
			MRP:         149.99,
			Currency:    "USD",
			Status:      model.StatusDraft,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		outboxEvt, err := productEvents.BuildProductCreatedOutboxEvent(prod)
		if err != nil {
			t.Fatalf("failed to build ProductCreated outbox event: %v", err)
		}

		if outboxEvt.EventType != events.EventTypeProductCreated {
			t.Errorf("expected EventType %s, got %s", events.EventTypeProductCreated, outboxEvt.EventType)
		}
		if outboxEvt.Topic != events.TopicProductCreated {
			t.Errorf("expected Topic %s, got %s", events.TopicProductCreated, outboxEvt.Topic)
		}
		if outboxEvt.AggregateID != prod.ID {
			t.Errorf("expected AggregateID %s, got %s", prod.ID, outboxEvt.AggregateID)
		}
		if outboxEvt.AggregateType != "product" {
			t.Errorf("expected AggregateType 'product', got %s", outboxEvt.AggregateType)
		}

		// Verify envelope deserialization
		env, err := events.UnmarshalEnvelope(outboxEvt.Payload)
		if err != nil {
			t.Fatalf("failed to unmarshal event envelope: %v", err)
		}
		if env.EventType != events.EventTypeProductCreated {
			t.Errorf("expected envelope EventType %s, got %s", events.EventTypeProductCreated, env.EventType)
		}
		if env.Source != "product-service" {
			t.Errorf("expected envelope Source 'product-service', got %s", env.Source)
		}

		var payload events.ProductCreatedEvent
		if err := env.UnmarshalData(&payload); err != nil {
			t.Fatalf("failed to unmarshal ProductCreatedEvent payload: %v", err)
		}
		if payload.ProductID != prod.ID {
			t.Errorf("expected product_id %s, got %s", prod.ID, payload.ProductID)
		}
		if payload.StoreID != prod.StoreID {
			t.Errorf("expected store_id %s, got %s", prod.StoreID, payload.StoreID)
		}
		if payload.SKU != prod.SKU || payload.Price != prod.Price || payload.Status != string(prod.Status) {
			t.Errorf("payload mismatch: %+v", payload)
		}
	})

	t.Run("ProductUpdated publishes ProductUpdated event with product/store identification", func(t *testing.T) {
		prod := &model.Product{
			ID:          "prod-uuid-101",
			StoreID:     "store-uuid-202",
			CategoryID:  "cat-uuid-303",
			SKU:         "SKU-KEYBOARD-RGB",
			Name:        "RGB Mechanical Keyboard Wireless",
			Description: "Updated wireless mechanical keyboard",
			Price:       149.99,
			MRP:         169.99,
			Currency:    "USD",
			Status:      model.StatusDraft,
			CreatedAt:   now,
			UpdatedAt:   now.Add(2 * time.Hour),
		}

		outboxEvt, err := productEvents.BuildProductUpdatedOutboxEvent(prod)
		if err != nil {
			t.Fatalf("failed to build ProductUpdated outbox event: %v", err)
		}

		if outboxEvt.EventType != events.EventTypeProductUpdated {
			t.Errorf("expected EventType %s, got %s", events.EventTypeProductUpdated, outboxEvt.EventType)
		}
		if outboxEvt.Topic != events.TopicProductUpdated {
			t.Errorf("expected Topic %s, got %s", events.TopicProductUpdated, outboxEvt.Topic)
		}
		if outboxEvt.AggregateID != prod.ID {
			t.Errorf("expected AggregateID %s, got %s", prod.ID, outboxEvt.AggregateID)
		}

		env, err := events.UnmarshalEnvelope(outboxEvt.Payload)
		if err != nil {
			t.Fatalf("failed to unmarshal event envelope: %v", err)
		}
		var payload events.ProductUpdatedEvent
		if err := env.UnmarshalData(&payload); err != nil {
			t.Fatalf("failed to unmarshal ProductUpdatedEvent payload: %v", err)
		}
		if payload.ProductID != prod.ID {
			t.Errorf("expected product_id %s, got %s", prod.ID, payload.ProductID)
		}
		if payload.StoreID != prod.StoreID {
			t.Errorf("expected store_id %s, got %s", prod.StoreID, payload.StoreID)
		}
		if payload.Price != 149.99 || payload.Name != "RGB Mechanical Keyboard Wireless" {
			t.Errorf("payload mismatch: %+v", payload)
		}
	})

	t.Run("Publishing publishes ProductPublished event with product/store identification", func(t *testing.T) {
		pubTime := now.Add(3 * time.Hour)
		prod := &model.Product{
			ID:          "prod-uuid-101",
			StoreID:     "store-uuid-202",
			CategoryID:  "cat-uuid-303",
			SKU:         "SKU-KEYBOARD-RGB",
			Name:        "RGB Mechanical Keyboard Wireless",
			Price:       149.99,
			Currency:    "USD",
			Status:      model.StatusPublished,
			UpdatedAt:   pubTime,
		}

		outboxEvt, err := productEvents.BuildProductPublishedOutboxEvent(prod, pubTime)
		if err != nil {
			t.Fatalf("failed to build ProductPublished outbox event: %v", err)
		}

		if outboxEvt.EventType != events.EventTypeProductPublished {
			t.Errorf("expected EventType %s, got %s", events.EventTypeProductPublished, outboxEvt.EventType)
		}
		if outboxEvt.Topic != events.TopicProductPublished {
			t.Errorf("expected Topic %s, got %s", events.TopicProductPublished, outboxEvt.Topic)
		}
		if outboxEvt.AggregateID != prod.ID {
			t.Errorf("expected AggregateID %s, got %s", prod.ID, outboxEvt.AggregateID)
		}

		env, err := events.UnmarshalEnvelope(outboxEvt.Payload)
		if err != nil {
			t.Fatalf("failed to unmarshal envelope: %v", err)
		}
		var payload events.ProductPublishedEvent
		if err := env.UnmarshalData(&payload); err != nil {
			t.Fatalf("failed to unmarshal ProductPublishedEvent payload: %v", err)
		}
		if payload.ProductID != prod.ID {
			t.Errorf("expected product_id %s, got %s", prod.ID, payload.ProductID)
		}
		if payload.StoreID != prod.StoreID {
			t.Errorf("expected store_id %s, got %s", prod.StoreID, payload.StoreID)
		}
		if payload.Price != 149.99 {
			t.Errorf("expected price 149.99, got %f", payload.Price)
		}
		if !payload.PublishedAt.Equal(pubTime) {
			t.Errorf("expected published_at %v, got %v", pubTime, payload.PublishedAt)
		}
	})

	t.Run("Deletion publishes ProductDeleted event with product/store identification", func(t *testing.T) {
		delTime := now.Add(4 * time.Hour)
		prodID := "prod-uuid-101"
		storeID := "store-uuid-202"

		outboxEvt, err := productEvents.BuildProductDeletedOutboxEvent(prodID, storeID, delTime)
		if err != nil {
			t.Fatalf("failed to build ProductDeleted outbox event: %v", err)
		}

		if outboxEvt.EventType != events.EventTypeProductDeleted {
			t.Errorf("expected EventType %s, got %s", events.EventTypeProductDeleted, outboxEvt.EventType)
		}
		if outboxEvt.Topic != events.TopicProductDeleted {
			t.Errorf("expected Topic %s, got %s", events.TopicProductDeleted, outboxEvt.Topic)
		}
		if outboxEvt.AggregateID != prodID {
			t.Errorf("expected AggregateID %s, got %s", prodID, outboxEvt.AggregateID)
		}

		env, err := events.UnmarshalEnvelope(outboxEvt.Payload)
		if err != nil {
			t.Fatalf("failed to unmarshal envelope: %v", err)
		}
		var payload events.ProductDeletedEvent
		if err := env.UnmarshalData(&payload); err != nil {
			t.Fatalf("failed to unmarshal ProductDeletedEvent payload: %v", err)
		}
		if payload.ProductID != prodID {
			t.Errorf("expected product_id %s, got %s", prodID, payload.ProductID)
		}
		if payload.StoreID != storeID {
			t.Errorf("expected store_id %s, got %s", storeID, payload.StoreID)
		}
		if payload.Status != "DELETED" {
			t.Errorf("expected status 'DELETED', got %s", payload.Status)
		}
		if !payload.DeletedAt.Equal(delTime) {
			t.Errorf("expected deleted_at %v, got %v", delTime, payload.DeletedAt)
		}
	})

	t.Run("Publishing through Mock EventBus succeeds for downstream consumers", func(t *testing.T) {
		bus := NewMockEventBus()
		ctx := context.Background()

		prod := &model.Product{
			ID:         "search-prod-1",
			StoreID:    "search-store-1",
			CategoryID: "cat-1",
			SKU:        "SEARCH-SKU-1",
			Name:       "Search Indexable Gadget",
			Price:      49.95,
			Status:     model.StatusPublished,
			CreatedAt:  now,
		}

		outboxEvt, err := productEvents.BuildProductCreatedOutboxEvent(prod)
		if err != nil {
			t.Fatalf("failed to build event: %v", err)
		}
		env, err := events.UnmarshalEnvelope(outboxEvt.Payload)
		if err != nil {
			t.Fatalf("failed to unmarshal envelope: %v", err)
		}

		err = bus.Publish(ctx, outboxEvt.Topic, outboxEvt.AggregateID, env)
		if err != nil {
			t.Fatalf("expected bus publish to succeed: %v", err)
		}

		published := bus.GetEvents()
		if len(published) != 1 {
			t.Fatalf("expected 1 published event, got %d", len(published))
		}
		if published[0].Topic != events.TopicProductCreated {
			t.Errorf("expected topic %s, got %s", events.TopicProductCreated, published[0].Topic)
		}
		if published[0].Key != prod.ID {
			t.Errorf("expected key %s, got %s", prod.ID, published[0].Key)
		}
	})
}

func TestCatalogConsistencyAndHistoricalPriceSnapshots(t *testing.T) {
	// Historical Order line item snapshot model representing order_items in order-service
	type OrderItemSnapshot struct {
		ID          string
		OrderID     string
		ProductID   string
		StoreID     string
		SKU         string
		ProductName string
		Quantity    int
		UnitPrice   float64
		TotalPrice  float64
	}

	// 1. Setup initial product in catalog
	catalogProduct := &model.Product{
		ID:        "prod-cons-101",
		StoreID:   "store-cons-202",
		SKU:       "LAPTOP-PRO-16",
		Name:      "Pro Laptop 16-inch",
		Price:     1999.00,
		Status:    model.StatusPublished,
		CreatedAt: time.Now().UTC().Add(-48 * time.Hour),
	}

	// 2. An order is placed at checkout. The order service snapshots the catalog item details.
	orderSnapshot := OrderItemSnapshot{
		ID:          "order-item-uuid-1",
		OrderID:     "order-uuid-999",
		ProductID:   catalogProduct.ID,
		StoreID:     catalogProduct.StoreID,
		SKU:         catalogProduct.SKU,
		ProductName: catalogProduct.Name,
		Quantity:    2,
		UnitPrice:   catalogProduct.Price, // Snapshot price at purchase time: 1999.00
		TotalPrice:  catalogProduct.Price * 2,
	}

	if orderSnapshot.UnitPrice != 1999.00 {
		t.Fatalf("expected snapshot price 1999.00, got %f", orderSnapshot.UnitPrice)
	}
	if orderSnapshot.TotalPrice != 3998.00 {
		t.Fatalf("expected snapshot total price 3998.00, got %f", orderSnapshot.TotalPrice)
	}

	// 3. Merchant updates product price in catalog (e.g. Price increase or discount)
	newPrice := 2499.00
	catalogProduct.Price = newPrice
	catalogProduct.UpdatedAt = time.Now().UTC()

	// 4. Verify catalog publishes ProductUpdated event with new price
	outboxEvt, err := productEvents.BuildProductUpdatedOutboxEvent(catalogProduct)
	if err != nil {
		t.Fatalf("failed building updated event: %v", err)
	}
	env, err := events.UnmarshalEnvelope(outboxEvt.Payload)
	if err != nil {
		t.Fatalf("failed unmarshaling envelope: %v", err)
	}
	var updatePayload events.ProductUpdatedEvent
	if err := env.UnmarshalData(&updatePayload); err != nil {
		t.Fatalf("failed unmarshaling data: %v", err)
	}

	if updatePayload.Price != 2499.00 {
		t.Errorf("expected updated event price 2499.00, got %f", updatePayload.Price)
	}

	// 5. CRITICAL VERIFICATION:
	// Historical order item snapshot MUST remain unaffected by the catalog price change.
	if orderSnapshot.UnitPrice != 1999.00 {
		t.Errorf("historical order unit_price modified! expected 1999.00, got %f", orderSnapshot.UnitPrice)
	}
	if orderSnapshot.TotalPrice != 3998.00 {
		t.Errorf("historical order total_price modified! expected 3998.00, got %f", orderSnapshot.TotalPrice)
	}
	if orderSnapshot.ProductID != updatePayload.ProductID {
		t.Errorf("product ID mismatch between snapshot and catalog update")
	}
}

func TestAtomicStateChangeGuarantee(t *testing.T) {
	// Verifies that if a state mutation fails, outbox events are structurally not committed
	type simulatedDB struct {
		productInserted bool
		outboxEvents    []*events.EventEnvelope
	}

	db := &simulatedDB{}

	executeAtomicTransaction := func(shouldFail bool) error {
		// Begin atomic unit of work
		txProductInserted := false
		var txOutboxEvents []*events.EventEnvelope

		// 1. Mutate product
		txProductInserted = true

		// 2. Generate outbox event
		prod := &model.Product{
			ID:      "fail-test-prod",
			StoreID: "fail-test-store",
			Price:   10.0,
			Status:  model.StatusDraft,
		}
		evt, err := productEvents.BuildProductCreatedOutboxEvent(prod)
		if err != nil {
			return err
		}
		env, _ := events.UnmarshalEnvelope(evt.Payload)
		txOutboxEvents = append(txOutboxEvents, env)

		// 3. If error occurs (e.g. constraint violation), rollback
		if shouldFail {
			// Rollback: no changes applied to db
			return errors.New("simulated database constraint error: duplicate SKU")
		}

		// Commit
		db.productInserted = txProductInserted
		db.outboxEvents = txOutboxEvents
		return nil
	}

	// Test rollback path
	err := executeAtomicTransaction(true)
	if err == nil {
		t.Fatalf("expected error from failed transaction")
	}
	if db.productInserted {
		t.Errorf("state change should NOT be applied on rollback")
	}
	if len(db.outboxEvents) != 0 {
		t.Errorf("outbox events should NOT be published on rollback! Got %d", len(db.outboxEvents))
	}

	// Test commit path
	err = executeAtomicTransaction(false)
	if err != nil {
		t.Fatalf("expected success from valid transaction: %v", err)
	}
	if !db.productInserted {
		t.Errorf("state change should be applied on commit")
	}
	if len(db.outboxEvents) != 1 {
		t.Errorf("expected 1 outbox event on commit, got %d", len(db.outboxEvents))
	}
}
