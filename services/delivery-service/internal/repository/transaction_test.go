package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
)

type MockTransactionalDeliveryRepository struct {
	mu           sync.Mutex
	deliveries   map[string]*model.Delivery
	trackingLogs []string
	failTracking bool
	failCommit   bool
	rollbackDone bool
}

func NewMockTransactionalDeliveryRepository() *MockTransactionalDeliveryRepository {
	return &MockTransactionalDeliveryRepository{
		deliveries:   make(map[string]*model.Delivery),
		trackingLogs: make([]string, 0),
	}
}

func (m *MockTransactionalDeliveryRepository) UpdateStatusWithTransaction(ctx context.Context, id string, targetStatus model.DeliveryStatus) (*model.Delivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	del, exists := m.deliveries[id]
	if !exists {
		return nil, errors.New("delivery not found")
	}

	origStatus := del.Status
	origUpdatedAt := del.UpdatedAt

	// 1. Validate state transition inside tx
	if err := del.TransitionStatus(targetStatus); err != nil {
		m.rollbackDone = true
		return nil, err
	}

	// 2. Insert tracking update
	if m.failTracking {
		del.Status = origStatus
		del.UpdatedAt = origUpdatedAt
		m.rollbackDone = true
		return nil, errors.New("failed to record tracking update: database error")
	}
	m.trackingLogs = append(m.trackingLogs, string(targetStatus))

	// 3. Commit
	if m.failCommit {
		del.Status = origStatus
		del.UpdatedAt = origUpdatedAt
		m.rollbackDone = true
		return nil, errors.New("failed to commit transaction")
	}

	m.rollbackDone = false
	return del, nil
}

func TestDeliveryTransactionRollback_OnTrackingFailure(t *testing.T) {
	repo := NewMockTransactionalDeliveryRepository()
	delID := uuid.NewString()
	initialDelivery := &model.Delivery{
		ID:        delID,
		OrderID:   uuid.NewString(),
		Status:    model.StatusCreated,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	repo.deliveries[delID] = initialDelivery
	repo.failTracking = true

	updated, err := repo.UpdateStatusWithTransaction(context.Background(), delID, model.StatusPickedUp)
	if err == nil {
		t.Fatal("expected error on tracking failure, but succeeded")
	}
	if updated != nil {
		t.Errorf("expected updated delivery to be nil on error, got %v", updated)
	}
	if !repo.rollbackDone {
		t.Errorf("expected rollback to be performed")
	}
	if repo.deliveries[delID].Status != model.StatusCreated {
		t.Errorf("expected status to remain CREATED, got %s", repo.deliveries[delID].Status)
	}
}

func TestDeliveryTransactionRollback_OnInvalidTransition(t *testing.T) {
	repo := NewMockTransactionalDeliveryRepository()
	delID := uuid.NewString()
	initialDelivery := &model.Delivery{
		ID:        delID,
		OrderID:   uuid.NewString(),
		Status:    model.StatusCreated,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	repo.deliveries[delID] = initialDelivery

	// CREATED -> DELIVERED is invalid
	updated, err := repo.UpdateStatusWithTransaction(context.Background(), delID, model.StatusDelivered)
	if err == nil {
		t.Fatal("expected error on invalid transition, but succeeded")
	}
	if updated != nil {
		t.Errorf("expected updated delivery to be nil, got %v", updated)
	}
	if !repo.rollbackDone {
		t.Errorf("expected rollback on invalid transition")
	}
	if repo.deliveries[delID].Status != model.StatusCreated {
		t.Errorf("expected status to remain CREATED, got %s", repo.deliveries[delID].Status)
	}
}
