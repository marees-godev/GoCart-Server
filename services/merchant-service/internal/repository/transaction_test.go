package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
)

// MockTransactionalRepository simulates database transactions with rollback capability
type MockTransactionalRepository struct {
	mu           sync.Mutex
	merchants    map[uuid.UUID]*model.Merchant
	audits       []*model.MerchantLifecycleAudit
	failAudit    bool
	failCommit   bool
	rollbackDone bool
}

func NewMockTransactionalRepository() *MockTransactionalRepository {
	return &MockTransactionalRepository{
		merchants: make(map[uuid.UUID]*model.Merchant),
		audits:    make([]*model.MerchantLifecycleAudit, 0),
	}
}

func (m *MockTransactionalRepository) UpdateStatusWithAudit(ctx context.Context, id uuid.UUID, newStatus model.MerchantStatus, reason string, updatedBy string) (*model.Merchant, model.MerchantStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	merch, exists := m.merchants[id]
	if !exists {
		return nil, "", errors.New("merchant not found")
	}

	// Snapshot for transaction simulation
	originalStatus := merch.Status
	originalReason := merch.RejectionReason
	originalUpdatedAt := merch.UpdatedAt

	currentStatus := model.MerchantStatus(originalStatus)

	// Step 1: Validate transition inside tx
	validator := model.NewStateTransitionValidator()
	if err := validator.Validate(currentStatus, newStatus); err != nil {
		m.rollbackDone = true
		return nil, currentStatus, err
	}

	// Step 2: Tentative update
	merch.Status = string(newStatus)
	merch.RejectionReason = reason
	merch.UpdatedAt = time.Now().UTC()

	// Step 3: Insert into audit table
	if m.failAudit {
		// Rollback merchant state to before transaction
		merch.Status = originalStatus
		merch.RejectionReason = originalReason
		merch.UpdatedAt = originalUpdatedAt
		m.rollbackDone = true
		return nil, currentStatus, errors.New("failed to record merchant status audit: database error")
	}

	audit := &model.MerchantLifecycleAudit{
		ID:             uuid.New(),
		MerchantID:     id,
		AdminID:        updatedBy,
		Action:         model.LifecycleActionApprove,
		PreviousStatus: string(currentStatus),
		NewStatus:      string(newStatus),
		Reason:         reason,
		Status:         model.AuditStatusSuccess,
		CreatedAt:      time.Now().UTC(),
	}
	m.audits = append(m.audits, audit)

	// Step 4: Commit tx
	if m.failCommit {
		merch.Status = originalStatus
		merch.RejectionReason = originalReason
		merch.UpdatedAt = originalUpdatedAt
		m.rollbackDone = true
		return nil, currentStatus, errors.New("failed to commit transaction")
	}

	m.rollbackDone = false
	return merch, currentStatus, nil
}

func TestTransactionRollback_OnAuditFailure(t *testing.T) {
	repo := NewMockTransactionalRepository()

	merchID := uuid.New()
	initialMerchant := &model.Merchant{
		ID:           merchID,
		BusinessName: "Transactional Test Store",
		Status:       string(model.MerchantStatusPending),
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	repo.merchants[merchID] = initialMerchant

	// Set audit insertion to FAIL
	repo.failAudit = true

	// Attempt update from PENDING to APPROVED
	updated, prevStatus, err := repo.UpdateStatusWithAudit(context.Background(), merchID, model.MerchantStatusApproved, "Audit failure test", "ADMIN")

	// 1. Must return error
	if err == nil {
		t.Fatal("expected error on audit failure, but update succeeded")
	}
	if updated != nil {
		t.Errorf("expected updated merchant to be nil on error, got %v", updated)
	}
	if prevStatus != model.MerchantStatusPending {
		t.Errorf("expected prevStatus PENDING, got %s", prevStatus)
	}

	// 2. Transaction must have rolled back
	if !repo.rollbackDone {
		t.Error("expected transaction rollback to be executed")
	}

	// 3. Database status must NOT be modified (still PENDING)
	persistedMerchant := repo.merchants[merchID]
	if persistedMerchant.Status != string(model.MerchantStatusPending) {
		t.Errorf("expected persisted merchant status to remain 'PENDING', but got %s", persistedMerchant.Status)
	}

	// 4. No audit record should exist
	if len(repo.audits) != 0 {
		t.Errorf("expected 0 audit records after rollback, got %d", len(repo.audits))
	}

	// Now retry with audit success
	repo.failAudit = false
	updatedSuccess, prevSuccess, err := repo.UpdateStatusWithAudit(context.Background(), merchID, model.MerchantStatusApproved, "KYC verified", "ADMIN")
	if err != nil {
		t.Fatalf("expected successful update when audit succeeds, got %v", err)
	}
	if updatedSuccess.Status != string(model.MerchantStatusApproved) {
		t.Errorf("expected status APPROVED, got %s", updatedSuccess.Status)
	}
	if prevSuccess != model.MerchantStatusPending {
		t.Errorf("expected previous status PENDING, got %s", prevSuccess)
	}
	if len(repo.audits) != 1 {
		t.Errorf("expected 1 audit record, got %d", len(repo.audits))
	}
	if repo.audits[0].PreviousStatus != string(model.MerchantStatusPending) || repo.audits[0].NewStatus != string(model.MerchantStatusApproved) {
		t.Errorf("audit record mismatch: from %s to %s", repo.audits[0].PreviousStatus, repo.audits[0].NewStatus)
	}
}

func TestTransactionRollback_OnInvalidStateTransition(t *testing.T) {
	repo := NewMockTransactionalRepository()

	merchID := uuid.New()
	initialMerchant := &model.Merchant{
		ID:           merchID,
		BusinessName: "Invalid Transition Test",
		Status:       string(model.MerchantStatusPending),
	}
	repo.merchants[merchID] = initialMerchant

	// Attempt invalid transition PENDING -> SUSPENDED
	_, _, err := repo.UpdateStatusWithAudit(context.Background(), merchID, model.MerchantStatusSuspended, "Invalid suspension", "ADMIN")
	if err == nil {
		t.Fatal("expected error on invalid transition PENDING -> SUSPENDED")
	}

	if !repo.rollbackDone {
		t.Error("expected transaction rollback on invalid transition")
	}

	// Status must remain PENDING
	if repo.merchants[merchID].Status != string(model.MerchantStatusPending) {
		t.Errorf("expected status to remain PENDING, got %s", repo.merchants[merchID].Status)
	}
	if len(repo.audits) != 0 {
		t.Errorf("expected 0 audit records, got %d", len(repo.audits))
	}
}
