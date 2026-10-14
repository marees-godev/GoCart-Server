package consumer_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/service"
)

type memoryDeliveryRepo struct {
	mu         sync.RWMutex
	deliveries map[string]*model.Delivery
	byOrderID  map[string]string
	byTracking map[string]string
}

func newMemoryDeliveryRepo() repository.DeliveryRepository {
	return &memoryDeliveryRepo{
		deliveries: make(map[string]*model.Delivery),
		byOrderID:  make(map[string]string),
		byTracking: make(map[string]string),
	}
}

func (r *memoryDeliveryRepo) Create(ctx context.Context, delivery *model.Delivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Enforce single-delivery-per-order at repository level
	if existingID, ok := r.byOrderID[delivery.OrderID]; ok {
		_ = existingID
		return nil // Idempotent no-op
	}

	if delivery.ID == "" {
		delivery.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = now
	}
	if delivery.UpdatedAt.IsZero() {
		delivery.UpdatedAt = now
	}

	stored := *delivery
	r.deliveries[delivery.ID] = &stored
	r.byOrderID[delivery.OrderID] = delivery.ID
	r.byTracking[delivery.TrackingNumber] = delivery.ID
	return nil
}

func (r *memoryDeliveryRepo) GetByID(ctx context.Context, id string) (*model.Delivery, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, ok := r.deliveries[id]
	if !ok {
		return nil, appErrors.NotFound("delivery not found")
	}
	copyD := *d
	return &copyD, nil
}

func (r *memoryDeliveryRepo) GetByOrderID(ctx context.Context, orderID string) (*model.Delivery, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.byOrderID[orderID]
	if !ok {
		return nil, appErrors.NotFound("delivery not found for order")
	}
	return r.GetByID(ctx, id)
}

func (r *memoryDeliveryRepo) GetByTrackingNumber(ctx context.Context, trackingNumber string) (*model.Delivery, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.byTracking[trackingNumber]
	if !ok {
		return nil, appErrors.NotFound("delivery not found for tracking number")
	}
	return r.GetByID(ctx, id)
}

func (r *memoryDeliveryRepo) List(ctx context.Context, limit, offset int, status string) ([]*model.Delivery, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Delivery
	for _, d := range r.deliveries {
		if status == "" || string(d.Status) == status {
			copyD := *d
			result = append(result, &copyD)
		}
	}
	total := len(result)
	return result, total, nil
}

func (r *memoryDeliveryRepo) UpdateStatus(ctx context.Context, id string, status model.DeliveryStatus) (*model.Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	d, ok := r.deliveries[id]
	if !ok {
		return nil, appErrors.NotFound("delivery not found")
	}
	d.Status = status
	d.UpdatedAt = time.Now().UTC()
	copyD := *d
	return &copyD, nil
}

func (r *memoryDeliveryRepo) UpsertTempOrder(ctx context.Context, order *model.TempOrder) error {
	return nil
}

func (r *memoryDeliveryRepo) GetTempOrderByID(ctx context.Context, id string) (*model.TempOrder, error) {
	return nil, appErrors.NotFound("temp order not found")
}

func TestOrderConfirmed_TriggersDeliveryCreation(t *testing.T) {
	repo := newMemoryDeliveryRepo()
	svc := service.NewDeliveryService(repo)
	eventConsumer := consumer.NewDeliveryEventConsumer(svc)

	orderID := "ord-" + uuid.NewString()
	shippingAddr := events.ShippingAddressPayload{
		Street:        "123 Market St",
		City:          "San Francisco",
		State:         "CA",
		PostalCode:    "94105",
		Country:       "USA",
		ContactNumber: "+1-555-0199",
	}

	payload := events.OrderConfirmedEvent{
		OrderID:         orderID,
		ShippingAddress: &shippingAddr,
		ConfirmedAt:     time.Now().UTC(),
	}

	envelope, err := events.NewEventEnvelope(events.EventTypeOrderConfirmed, "order-service", payload)
	if err != nil {
		t.Fatalf("failed to create EventEnvelope: %v", err)
	}

	ctx := context.Background()
	if err := eventConsumer.HandleEvent(ctx, envelope); err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	delivery, err := svc.GetDeliveryByOrderID(ctx, orderID)
	if err != nil {
		t.Fatalf("GetDeliveryByOrderID failed: %v", err)
	}

	if delivery.OrderID != orderID {
		t.Errorf("expected order_id %s, got %s", orderID, delivery.OrderID)
	}
	if delivery.Status != model.StatusCreated {
		t.Errorf("expected initial status CREATED, got %s", delivery.Status)
	}
	if delivery.DeliveryPartner == "" {
		t.Errorf("expected delivery_partner to be assigned, got empty")
	}
	if delivery.TrackingNumber == "" {
		t.Errorf("expected tracking_number to be generated, got empty")
	}
	if delivery.ShippingAddress.Street != "123 Market St" {
		t.Errorf("expected street '123 Market St', got %s", delivery.ShippingAddress.Street)
	}
	if delivery.ShippingAddress.City != "San Francisco" {
		t.Errorf("expected city 'San Francisco', got %s", delivery.ShippingAddress.City)
	}
	if delivery.ShippingAddress.PostalCode != "94105" {
		t.Errorf("expected postal_code '94105', got %s", delivery.ShippingAddress.PostalCode)
	}
	if delivery.ShippingAddress.ContactNumber != "+1-555-0199" {
		t.Errorf("expected contact_number '+1-555-0199', got %s", delivery.ShippingAddress.ContactNumber)
	}
}

func TestOrderConfirmed_Idempotency_DuplicateEvents(t *testing.T) {
	repo := newMemoryDeliveryRepo()
	svc := service.NewDeliveryService(repo)
	eventConsumer := consumer.NewDeliveryEventConsumer(svc)

	orderID := "ord-dup-" + uuid.NewString()
	shippingAddr := events.ShippingAddressPayload{
		Street:        "456 Elm St",
		City:          "Seattle",
		State:         "WA",
		PostalCode:    "98101",
		Country:       "USA",
		ContactNumber: "+1-555-0888",
	}

	payload := events.OrderConfirmedEvent{
		OrderID:         orderID,
		ShippingAddress: &shippingAddr,
		ConfirmedAt:     time.Now().UTC(),
	}

	ctx := context.Background()

	// 1st OrderConfirmed event
	env1, err := events.NewEventEnvelope(events.EventTypeOrderConfirmed, "order-service", payload)
	if err != nil {
		t.Fatalf("failed to create 1st EventEnvelope: %v", err)
	}
	if err := eventConsumer.HandleEvent(ctx, env1); err != nil {
		t.Fatalf("1st OrderConfirmed event failed: %v", err)
	}

	firstDelivery, err := svc.GetDeliveryByOrderID(ctx, orderID)
	if err != nil {
		t.Fatalf("GetDeliveryByOrderID failed after 1st event: %v", err)
	}

	// 2nd duplicate OrderConfirmed event
	env2, err := events.NewEventEnvelope(events.EventTypeOrderConfirmed, "order-service", payload)
	if err != nil {
		t.Fatalf("failed to create 2nd EventEnvelope: %v", err)
	}
	if err := eventConsumer.HandleEvent(ctx, env2); err != nil {
		t.Fatalf("2nd duplicate OrderConfirmed event failed: %v", err)
	}

	// 3rd duplicate OrderConfirmed event
	env3, err := events.NewEventEnvelope(events.EventTypeOrderConfirmed, "order-service", payload)
	if err != nil {
		t.Fatalf("failed to create 3rd EventEnvelope: %v", err)
	}
	if err := eventConsumer.HandleEvent(ctx, env3); err != nil {
		t.Fatalf("3rd duplicate OrderConfirmed event failed: %v", err)
	}

	secondDelivery, err := svc.GetDeliveryByOrderID(ctx, orderID)
	if err != nil {
		t.Fatalf("GetDeliveryByOrderID failed after duplicate events: %v", err)
	}

	if firstDelivery.ID != secondDelivery.ID {
		t.Errorf("expected same delivery ID, got %s vs %s", firstDelivery.ID, secondDelivery.ID)
	}
	if firstDelivery.TrackingNumber != secondDelivery.TrackingNumber {
		t.Errorf("expected same tracking number, got %s vs %s", firstDelivery.TrackingNumber, secondDelivery.TrackingNumber)
	}
}

func TestOrderConfirmed_MissingContactNumber_ReturnsError(t *testing.T) {
	repo := newMemoryDeliveryRepo()
	svc := service.NewDeliveryService(repo)
	eventConsumer := consumer.NewDeliveryEventConsumer(svc)

	orderID := "ord-no-contact-" + uuid.NewString()
	shippingAddr := events.ShippingAddressPayload{
		Street:     "123 Main St",
		City:       "Dallas",
		State:      "TX",
		PostalCode: "75001",
		Country:    "USA",
	}

	payload := events.OrderConfirmedEvent{
		OrderID:         orderID,
		ShippingAddress: &shippingAddr,
		ConfirmedAt:     time.Now().UTC(),
	}

	env, err := events.NewEventEnvelope(events.EventTypeOrderConfirmed, "order-service", payload)
	if err != nil {
		t.Fatalf("failed to create EventEnvelope: %v", err)
	}

	err = eventConsumer.HandleEvent(context.Background(), env)
	if err == nil {
		t.Fatalf("expected error when contact_number is missing, got nil")
	}
}
