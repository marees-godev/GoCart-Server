package consumer_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/service"
)

type memoryInventoryRepo struct {
	mu           sync.RWMutex
	inventories  map[string]*model.Inventory
	reservations map[string]*model.InventoryReservation
	transactions []*model.InventoryTransaction
}

func newMemoryInventoryRepo() repository.InventoryRepository {
	return &memoryInventoryRepo{
		inventories:  make(map[string]*model.Inventory),
		reservations: make(map[string]*model.InventoryReservation),
		transactions: make([]*model.InventoryTransaction, 0),
	}
}

func (r *memoryInventoryRepo) Create(ctx context.Context, inv *model.Inventory, initialQuantity int, notes string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if inv.ID == "" {
		inv.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	inv.CreatedAt = now
	inv.UpdatedAt = now

	stored := *inv
	r.inventories[inv.ID] = &stored
	return nil
}

func (r *memoryInventoryRepo) GetByID(ctx context.Context, id string) (*model.Inventory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	inv, ok := r.inventories[id]
	if !ok {
		return nil, appErrors.NotFound("inventory not found")
	}
	copyInv := *inv
	return &copyInv, nil
}

func (r *memoryInventoryRepo) GetByProductAndVariant(ctx context.Context, productID string, variantID *string) (*model.Inventory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, inv := range r.inventories {
		if inv.ProductID == productID {
			if variantID == nil || *variantID == "" {
				if inv.VariantID == nil {
					copyInv := *inv
					return &copyInv, nil
				}
			} else if inv.VariantID != nil && *inv.VariantID == *variantID {
				copyInv := *inv
				return &copyInv, nil
			}
		}
	}
	return nil, appErrors.NotFound("inventory not found for product/variant")
}

func (r *memoryInventoryRepo) Restock(ctx context.Context, id string, quantity int, refID, notes *string) (*model.Inventory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	inv, ok := r.inventories[id]
	if !ok {
		return nil, appErrors.NotFound("inventory not found")
	}
	inv.AvailableQuantity += quantity
	inv.UpdatedAt = time.Now().UTC()
	copyInv := *inv
	return &copyInv, nil
}

func (r *memoryInventoryRepo) UpdateStock(ctx context.Context, input dto.UpdateStockInput) (*model.Inventory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, inv := range r.inventories {
		if input.ProductID != nil && inv.ProductID == *input.ProductID {
			inv.AvailableQuantity = input.Quantity
			inv.UpdatedAt = time.Now().UTC()
			copyInv := *inv
			return &copyInv, nil
		}
	}
	return nil, appErrors.NotFound("inventory not found")
}

func (r *memoryInventoryRepo) ReserveStock(ctx context.Context, orderID string, items []model.ReserveItem, expiresAt time.Time) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, res := range r.reservations {
		if res.OrderID == orderID && (res.Status == model.ReservationStatusReserved || res.Status == model.ReservationStatusConfirmed) {
			return res.ID, nil
		}
	}

	resID := uuid.NewString()
	now := time.Now().UTC()

	for _, item := range items {
		var found *model.Inventory
		for _, inv := range r.inventories {
			if inv.ProductID == item.ProductID {
				found = inv
				break
			}
		}
		if found == nil {
			return "", appErrors.NotFound(fmt.Sprintf("inventory not found for product %s", item.ProductID))
		}
		if found.AvailableQuantity < item.Quantity {
			return "", appErrors.Conflict("insufficient stock")
		}
		found.AvailableQuantity -= item.Quantity
		found.ReservedQuantity += item.Quantity
		found.UpdatedAt = now

		r.reservations[resID] = &model.InventoryReservation{
			ID:        resID,
			OrderID:   orderID,
			ProductID: item.ProductID,
			VariantID: item.VariantID,
			Quantity:  item.Quantity,
			Status:    model.ReservationStatusReserved,
			ExpiresAt: expiresAt,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	return resID, nil
}

func (r *memoryInventoryRepo) ReleaseStock(ctx context.Context, input dto.ReleaseStockInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var matching []*model.InventoryReservation
	for _, res := range r.reservations {
		if (input.ReservationID != "" && res.ID == input.ReservationID) ||
			(input.OrderID != "" && res.OrderID == input.OrderID) {
			matching = append(matching, res)
		}
	}

	if len(matching) == 0 {
		return appErrors.NotFound("reservation not found")
	}

	targetStatus := model.ReservationStatusReleased
	if strings.EqualFold(input.Reason, "EXPIRED") {
		targetStatus = model.ReservationStatusExpired
	}

	now := time.Now().UTC()
	for _, res := range matching {
		if res.Status == model.ReservationStatusReserved {
			for _, inv := range r.inventories {
				if inv.ProductID == res.ProductID {
					inv.AvailableQuantity += res.Quantity
					inv.ReservedQuantity -= res.Quantity
					if inv.ReservedQuantity < 0 {
						inv.ReservedQuantity = 0
					}
					inv.UpdatedAt = now
					break
				}
			}
			res.Status = targetStatus
			res.UpdatedAt = now
		}
	}
	return nil
}

func (r *memoryInventoryRepo) CommitStock(ctx context.Context, input dto.CommitStockInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var matching []*model.InventoryReservation
	var confirmedCount int
	var releasedCount int

	for _, res := range r.reservations {
		if (input.ReservationID != "" && res.ID == input.ReservationID) ||
			(input.OrderID != "" && res.OrderID == input.OrderID) {
			matching = append(matching, res)
			if res.Status == model.ReservationStatusConfirmed {
				confirmedCount++
			} else if res.Status == model.ReservationStatusReleased || res.Status == model.ReservationStatusExpired {
				releasedCount++
			}
		}
	}

	if len(matching) == 0 {
		return appErrors.NotFound("reservation not found")
	}

	var active []*model.InventoryReservation
	for _, res := range matching {
		if res.Status == model.ReservationStatusReserved {
			active = append(active, res)
		}
	}

	if len(active) == 0 {
		if confirmedCount > 0 {
			return nil // Idempotent
		}
		if releasedCount > 0 {
			return appErrors.Conflict("cannot commit released or expired reservation")
		}
		return appErrors.Conflict("reservation is not in a committable state")
	}

	now := time.Now().UTC()
	for _, res := range active {
		for _, inv := range r.inventories {
			if inv.ProductID == res.ProductID {
				inv.ReservedQuantity -= res.Quantity
				if inv.ReservedQuantity < 0 {
					inv.ReservedQuantity = 0
				}
				inv.UpdatedAt = now
				break
			}
		}
		res.Status = model.ReservationStatusConfirmed
		res.UpdatedAt = now
	}
	return nil
}

func (r *memoryInventoryRepo) ReleaseExpiredReservations(ctx context.Context) (int, error) {
	r.mu.Lock()
	now := time.Now().UTC()
	var expiredIDs []string
	for _, res := range r.reservations {
		if res.Status == model.ReservationStatusReserved && !res.ExpiresAt.After(now) {
			expiredIDs = append(expiredIDs, res.ID)
		}
	}
	r.mu.Unlock()

	released := 0
	for _, id := range expiredIDs {
		err := r.ReleaseStock(ctx, dto.ReleaseStockInput{
			ReservationID: id,
			Reason:        "EXPIRED",
		})
		if err == nil {
			released++
		}
	}
	return released, nil
}

func (r *memoryInventoryRepo) GetTransactionsByInventoryID(ctx context.Context, inventoryID string, limit, offset int) ([]*model.InventoryTransaction, error) {
	return nil, nil
}

func setupConsumerTest() (*consumer.InventoryEventConsumer, repository.InventoryRepository, service.InventoryService) {
	repo := newMemoryInventoryRepo()
	svc := service.NewInventoryService(repo, nil)
	evtConsumer := consumer.NewInventoryEventConsumer(svc)
	return evtConsumer, repo, svc
}

func TestOrderCreated_TriggersStockReservation(t *testing.T) {
	evtConsumer, repo, _ := setupConsumerTest()
	prodID := uuid.NewString()

	_ = repo.Create(context.Background(), &model.Inventory{
		ProductID:         prodID,
		SKU:               "SKU-ORDER-CREATED",
		AvailableQuantity: 50,
	}, 50, "")

	orderID := uuid.NewString()
	payload := events.OrderCreatedEvent{
		OrderID: orderID,
		UserID:  uuid.NewString(),
		Items: []events.OrderItemPayload{
			{ProductID: prodID, Quantity: 10, UnitPrice: 15.0},
		},
		CreatedAt: time.Now().UTC(),
	}

	envelope, err := events.NewEventEnvelope(events.EventTypeOrderCreated, "order-service", payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}

	err = evtConsumer.HandleEvent(context.Background(), envelope)
	if err != nil {
		t.Fatalf("HandleEvent failed for OrderCreated: %v", err)
	}

	inv, err := repo.GetByProductAndVariant(context.Background(), prodID, nil)
	if err != nil {
		t.Fatalf("GetByProductAndVariant failed: %v", err)
	}
	if inv.AvailableQuantity != 40 {
		t.Errorf("expected available quantity 40, got %d", inv.AvailableQuantity)
	}
	if inv.ReservedQuantity != 10 {
		t.Errorf("expected reserved quantity 10, got %d", inv.ReservedQuantity)
	}
}

func TestPaymentFailed_TriggersStockRelease(t *testing.T) {
	evtConsumer, repo, svc := setupConsumerTest()
	prodID := uuid.NewString()

	_ = repo.Create(context.Background(), &model.Inventory{
		ProductID:         prodID,
		SKU:               "SKU-PAYMENT-FAILED",
		AvailableQuantity: 50,
	}, 50, "")

	orderID := uuid.NewString()
	_, _, err := svc.ReserveStock(context.Background(), dto.ReserveStockInput{
		OrderID: orderID,
		Items: []dto.ReserveItemInput{
			{ProductID: prodID, Quantity: 20},
		},
	})
	if err != nil {
		t.Fatalf("ReserveStock failed: %v", err)
	}

	payload := events.PaymentFailedEvent{
		PaymentID: uuid.NewString(),
		OrderID:   orderID,
		Reason:    "CARD_DECLINED",
		FailedAt:  time.Now().UTC(),
	}

	envelope, err := events.NewEventEnvelope(events.EventTypePaymentFailed, "payment-service", payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}

	err = evtConsumer.HandleEvent(context.Background(), envelope)
	if err != nil {
		t.Fatalf("HandleEvent failed for PaymentFailed: %v", err)
	}

	inv, err := repo.GetByProductAndVariant(context.Background(), prodID, nil)
	if err != nil {
		t.Fatalf("GetByProductAndVariant failed: %v", err)
	}
	if inv.AvailableQuantity != 50 {
		t.Errorf("expected available quantity restored to 50, got %d", inv.AvailableQuantity)
	}
	if inv.ReservedQuantity != 0 {
		t.Errorf("expected reserved quantity 0, got %d", inv.ReservedQuantity)
	}
}

func TestOrderCancelled_TriggersStockRelease(t *testing.T) {
	evtConsumer, repo, svc := setupConsumerTest()
	prodID := uuid.NewString()

	_ = repo.Create(context.Background(), &model.Inventory{
		ProductID:         prodID,
		SKU:               "SKU-ORDER-CANCELLED",
		AvailableQuantity: 30,
	}, 30, "")

	orderID := uuid.NewString()
	_, _, err := svc.ReserveStock(context.Background(), dto.ReserveStockInput{
		OrderID: orderID,
		Items: []dto.ReserveItemInput{
			{ProductID: prodID, Quantity: 15},
		},
	})
	if err != nil {
		t.Fatalf("ReserveStock failed: %v", err)
	}

	payload := events.OrderCancelledEvent{
		OrderID:     orderID,
		Reason:      "USER_CANCELLED",
		CancelledAt: time.Now().UTC(),
	}

	envelope, err := events.NewEventEnvelope(events.EventTypeOrderCancelled, "order-service", payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}

	err = evtConsumer.HandleEvent(context.Background(), envelope)
	if err != nil {
		t.Fatalf("HandleEvent failed for OrderCancelled: %v", err)
	}

	inv, err := repo.GetByProductAndVariant(context.Background(), prodID, nil)
	if err != nil {
		t.Fatalf("GetByProductAndVariant failed: %v", err)
	}
	if inv.AvailableQuantity != 30 {
		t.Errorf("expected available quantity restored to 30, got %d", inv.AvailableQuantity)
	}
}

func TestOrderConfirmed_TriggersStockCommit(t *testing.T) {
	evtConsumer, repo, svc := setupConsumerTest()
	prodID := uuid.NewString()

	_ = repo.Create(context.Background(), &model.Inventory{
		ProductID:         prodID,
		SKU:               "SKU-ORDER-CONFIRMED",
		AvailableQuantity: 40,
	}, 40, "")

	orderID := uuid.NewString()
	_, _, err := svc.ReserveStock(context.Background(), dto.ReserveStockInput{
		OrderID: orderID,
		Items: []dto.ReserveItemInput{
			{ProductID: prodID, Quantity: 10},
		},
	})
	if err != nil {
		t.Fatalf("ReserveStock failed: %v", err)
	}

	payload := events.OrderConfirmedEvent{
		OrderID:     orderID,
		ConfirmedAt: time.Now().UTC(),
	}

	envelope, err := events.NewEventEnvelope(events.EventTypeOrderConfirmed, "order-service", payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}

	err = evtConsumer.HandleEvent(context.Background(), envelope)
	if err != nil {
		t.Fatalf("HandleEvent failed for OrderConfirmed: %v", err)
	}

	inv, err := repo.GetByProductAndVariant(context.Background(), prodID, nil)
	if err != nil {
		t.Fatalf("GetByProductAndVariant failed: %v", err)
	}
	if inv.AvailableQuantity != 30 {
		t.Errorf("expected available quantity 30, got %d", inv.AvailableQuantity)
	}
	if inv.ReservedQuantity != 0 {
		t.Errorf("expected reserved quantity 0 after commit, got %d", inv.ReservedQuantity)
	}
}

func TestEventConsumers_AreIdempotent(t *testing.T) {
	evtConsumer, repo, _ := setupConsumerTest()
	prodID := uuid.NewString()

	_ = repo.Create(context.Background(), &model.Inventory{
		ProductID:         prodID,
		SKU:               "SKU-IDEMPOTENT",
		AvailableQuantity: 50,
	}, 50, "")

	orderID := uuid.NewString()
	createdPayload := events.OrderCreatedEvent{
		OrderID: orderID,
		Items: []events.OrderItemPayload{
			{ProductID: prodID, Quantity: 10},
		},
	}
	envCreated, _ := events.NewEventEnvelope(events.EventTypeOrderCreated, "order-service", createdPayload)

	// 1st OrderCreated
	err := evtConsumer.HandleEvent(context.Background(), envCreated)
	if err != nil {
		t.Fatalf("1st OrderCreated failed: %v", err)
	}

	// 2nd duplicate OrderCreated
	err = evtConsumer.HandleEvent(context.Background(), envCreated)
	if err != nil {
		t.Fatalf("2nd duplicate OrderCreated failed: %v", err)
	}

	inv, _ := repo.GetByProductAndVariant(context.Background(), prodID, nil)
	if inv.AvailableQuantity != 40 {
		t.Errorf("expected available quantity 40 after duplicate OrderCreated, got %d", inv.AvailableQuantity)
	}

	// 1st OrderConfirmed
	confirmedPayload := events.OrderConfirmedEvent{OrderID: orderID}
	envConfirmed, _ := events.NewEventEnvelope(events.EventTypeOrderConfirmed, "order-service", confirmedPayload)

	err = evtConsumer.HandleEvent(context.Background(), envConfirmed)
	if err != nil {
		t.Fatalf("1st OrderConfirmed failed: %v", err)
	}

	// 2nd duplicate OrderConfirmed
	err = evtConsumer.HandleEvent(context.Background(), envConfirmed)
	if err != nil {
		t.Fatalf("2nd duplicate OrderConfirmed failed: %v", err)
	}

	inv, _ = repo.GetByProductAndVariant(context.Background(), prodID, nil)
	if inv.AvailableQuantity != 40 {
		t.Errorf("expected available quantity 40 after duplicate OrderConfirmed, got %d", inv.AvailableQuantity)
	}
	if inv.ReservedQuantity != 0 {
		t.Errorf("expected reserved quantity 0 after duplicate OrderConfirmed, got %d", inv.ReservedQuantity)
	}
}

func TestReservationExpiry(t *testing.T) {
	_, repo, svc := setupConsumerTest()
	prodID := uuid.NewString()

	_ = repo.Create(context.Background(), &model.Inventory{
		ProductID:         prodID,
		SKU:               "SKU-EXPIRY",
		AvailableQuantity: 25,
	}, 25, "")

	orderID := uuid.NewString()
	// Reserve stock with negative/expired duration to simulate expiration
	expiresAt := time.Now().UTC().Add(-1 * time.Minute)
	_, err := repo.ReserveStock(context.Background(), orderID, []model.ReserveItem{
		{ProductID: prodID, Quantity: 10},
	}, expiresAt)
	if err != nil {
		t.Fatalf("ReserveStock failed: %v", err)
	}

	inv, _ := repo.GetByProductAndVariant(context.Background(), prodID, nil)
	if inv.AvailableQuantity != 15 {
		t.Errorf("expected available quantity 15 before release expired, got %d", inv.AvailableQuantity)
	}

	releasedCount, err := svc.ReleaseExpiredReservations(context.Background())
	if err != nil {
		t.Fatalf("ReleaseExpiredReservations failed: %v", err)
	}
	if releasedCount != 1 {
		t.Errorf("expected 1 released expired reservation, got %d", releasedCount)
	}

	invAfter, _ := repo.GetByProductAndVariant(context.Background(), prodID, nil)
	if invAfter.AvailableQuantity != 25 {
		t.Errorf("expected available quantity restored to 25, got %d", invAfter.AvailableQuantity)
	}
}

func TestReleaseCommitRaceCondition(t *testing.T) {
	_, repo, svc := setupConsumerTest()
	prodID := uuid.NewString()

	_ = repo.Create(context.Background(), &model.Inventory{
		ProductID:         prodID,
		SKU:               "SKU-RACE",
		AvailableQuantity: 40,
	}, 40, "")

	orderID := uuid.NewString()
	_, _, err := svc.ReserveStock(context.Background(), dto.ReserveStockInput{
		OrderID: orderID,
		Items: []dto.ReserveItemInput{
			{ProductID: prodID, Quantity: 20},
		},
	})
	if err != nil {
		t.Fatalf("ReserveStock failed: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	var commitErr, releaseErr error
	go func() {
		defer wg.Done()
		commitErr = svc.CommitStock(context.Background(), dto.CommitStockInput{OrderID: orderID})
	}()

	go func() {
		defer wg.Done()
		releaseErr = svc.ReleaseStock(context.Background(), dto.ReleaseStockInput{OrderID: orderID, Reason: "CANCELLED"})
	}()

	wg.Wait()

	// One operation must win, and system state must remain consistent
	if commitErr != nil && releaseErr != nil {
		t.Fatalf("both commit and release failed unexpectedly: commitErr=%v, releaseErr=%v", commitErr, releaseErr)
	}

	inv, _ := repo.GetByProductAndVariant(context.Background(), prodID, nil)
	if inv.ReservedQuantity != 0 {
		t.Errorf("expected reserved quantity 0 after race condition, got %d", inv.ReservedQuantity)
	}
	if inv.AvailableQuantity != 20 && inv.AvailableQuantity != 40 {
		t.Errorf("expected available quantity to be either 20 (commit won) or 40 (release won), got %d", inv.AvailableQuantity)
	}
}

func TestEventConsumer_MalformedPayload_ReturnsError(t *testing.T) {
	evtConsumer, _, _ := setupConsumerTest()

	// Envelope with invalid/corrupted JSON data payload
	envelope := &events.EventEnvelope{
		EventID:   uuid.NewString(),
		EventType: events.EventTypeOrderCreated,
		Data:      []byte(`{invalid json syntax`),
	}

	err := evtConsumer.HandleEvent(context.Background(), envelope)
	if err == nil {
		t.Fatalf("expected error for malformed JSON envelope data, got nil")
	}
}

func TestEventConsumer_InsufficientStock_HandledCleanly(t *testing.T) {
	evtConsumer, repo, _ := setupConsumerTest()
	prodID := uuid.NewString()

	_ = repo.Create(context.Background(), &model.Inventory{
		ProductID:         prodID,
		SKU:               "SKU-INSUFFICIENT",
		AvailableQuantity: 5,
	}, 5, "")

	orderID := uuid.NewString()
	payload := events.OrderCreatedEvent{
		OrderID: orderID,
		Items: []events.OrderItemPayload{
			{ProductID: prodID, Quantity: 20}, // Request 20 when only 5 available
		},
	}

	envelope, _ := events.NewEventEnvelope(events.EventTypeOrderCreated, "order-service", payload)
	_ = evtConsumer.HandleEvent(context.Background(), envelope)

	// In consumer, insufficient stock conflict is handled without crashing (or returns conflict error for retries)
	inv, _ := repo.GetByProductAndVariant(context.Background(), prodID, nil)
	if inv.AvailableQuantity != 5 {
		t.Errorf("expected available quantity to remain 5, got %d", inv.AvailableQuantity)
	}
	if inv.ReservedQuantity != 0 {
		t.Errorf("expected reserved quantity to remain 0, got %d", inv.ReservedQuantity)
	}
}
