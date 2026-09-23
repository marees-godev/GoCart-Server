package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/service"
)

type mockMerchantRepository struct {
	mu            sync.Mutex
	merchantsByID map[uuid.UUID]*model.Merchant
	createErr     error
}

func newMockRepo() *mockMerchantRepository {
	return &mockMerchantRepository{
		merchantsByID: make(map[uuid.UUID]*model.Merchant),
	}
}

func (m *mockMerchantRepository) Create(ctx context.Context, merchant *model.Merchant) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.createErr != nil {
		return m.createErr
	}

	if existing, exists := m.merchantsByID[merchant.ID]; exists {
		*merchant = *existing
		return nil
	}

	if merchant.ID == uuid.Nil {
		merchant.ID = uuid.New()
	}
	merchant.CreatedAt = time.Now().UTC()
	merchant.UpdatedAt = time.Now().UTC()

	copied := *merchant
	m.merchantsByID[merchant.ID] = &copied
	return nil
}

func (m *mockMerchantRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Merchant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	merchant, exists := m.merchantsByID[id]
	if !exists {
		return nil, appErrors.NotFound("merchant not found")
	}
	copied := *merchant
	return &copied, nil
}

func (m *mockMerchantRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*model.Merchant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, merch := range m.merchantsByID {
		if merch.UserID == userID {
			copied := *merch
			return &copied, nil
		}
	}
	return nil, appErrors.NotFound("merchant not found")
}

func (m *mockMerchantRepository) List(ctx context.Context, limit, offset int, status string) ([]*model.Merchant, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []*model.Merchant
	for _, merch := range m.merchantsByID {
		if status == "" || merch.Status == status {
			c := *merch
			list = append(list, &c)
		}
	}
	return list, len(list), nil
}

func (m *mockMerchantRepository) Update(ctx context.Context, merchant *model.Merchant) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.merchantsByID[merchant.ID]; !exists {
		return appErrors.NotFound("merchant not found")
	}
	merchant.UpdatedAt = time.Now().UTC()
	c := *merchant
	m.merchantsByID[merchant.ID] = &c
	return nil
}

func (m *mockMerchantRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, rejectionReason string) (*model.Merchant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	merch, exists := m.merchantsByID[id]
	if !exists {
		return nil, appErrors.NotFound("merchant not found")
	}
	merch.Status = status
	merch.RejectionReason = rejectionReason
	merch.UpdatedAt = time.Now().UTC()
	c := *merch
	m.merchantsByID[id] = &c
	return &c, nil
}

func (m *mockMerchantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.merchantsByID[id]; !exists {
		return appErrors.NotFound("merchant not found")
	}
	delete(m.merchantsByID, id)
	return nil
}

func TestCreateMerchant_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	email := "test@merchant.com"
	phone := "+1234567890"
	taxID := "TAX-123"

	req := dto.CreateMerchantRequest{
		ID:            uuid.New().String(),
		BusinessName:  "Acme Corp",
		FirstName:     "Jane",
		LastName:      "Smith",
		BusinessEmail: email,
		BusinessPhone: phone,
		TaxID:         taxID,
	}

	merch, err := svc.CreateMerchant(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateMerchant failed: %v", err)
	}

	if merch.Status != string(model.MerchantStatusPending) {
		t.Errorf("expected initial status to be PENDING, got %s", merch.Status)
	}
	if merch.BusinessName != "Acme Corp" {
		t.Errorf("expected Acme Corp, got %s", merch.BusinessName)
	}
	if merch.FirstName != "Jane" || merch.LastName != "Smith" {
		t.Errorf("expected Jane Smith, got %s %s", merch.FirstName, merch.LastName)
	}
	if merch.BusinessEmail != email {
		t.Errorf("expected %s, got %s", email, merch.BusinessEmail)
	}
}

func TestCreateMerchant_Validation(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	// Missing BusinessName
	_, err := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID: uuid.New().String(),
	})
	if err == nil {
		t.Fatal("expected error for missing business name")
	}

	// Invalid ID format
	_, err = svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:           "not-a-uuid",
		BusinessName: "Invalid UUID",
	})
	if err == nil {
		t.Fatal("expected error for invalid UUID")
	}
}

func TestCreateMerchant_DuplicateID(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	uID := uuid.New().String()
	_, err := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:           uID,
		BusinessName: "First",
	})
	if err != nil {
		t.Fatalf("first creation failed: %v", err)
	}

	_, err = svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:           uID,
		BusinessName: "Second",
	})
	if err == nil {
		t.Fatal("expected conflict error for duplicate merchant ID")
	}
}

func TestUpdateMerchantStatus_ValidStatuses(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	merch, err := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:           uuid.New().String(),
		BusinessName: "Status Merchant",
	})
	if err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	// 1. Update to APPROVED
	updated, err := svc.UpdateMerchantStatus(context.Background(), merch.ID, dto.UpdateMerchantStatusRequest{
		Status: "APPROVED",
	})
	if err != nil {
		t.Fatalf("expected APPROVED to succeed, got %v", err)
	}
	if updated.Status != "APPROVED" {
		t.Errorf("expected APPROVED, got %s", updated.Status)
	}

	// 2. Update to REJECTED with reason
	reason := "Incomplete documentation"
	updated, err = svc.UpdateMerchantStatus(context.Background(), merch.ID, dto.UpdateMerchantStatusRequest{
		Status:          "REJECTED",
		RejectionReason: reason,
	})
	if err != nil {
		t.Fatalf("expected REJECTED to succeed, got %v", err)
	}
	if updated.Status != "REJECTED" {
		t.Errorf("expected REJECTED, got %s", updated.Status)
	}
	if updated.RejectionReason != reason {
		t.Errorf("expected reason %s, got %s", reason, updated.RejectionReason)
	}

	// 3. Update to PENDING
	updated, err = svc.UpdateMerchantStatus(context.Background(), merch.ID, dto.UpdateMerchantStatusRequest{
		Status: "PENDING",
	})
	if err != nil {
		t.Fatalf("expected PENDING to succeed, got %v", err)
	}
	if updated.Status != "PENDING" {
		t.Errorf("expected PENDING, got %s", updated.Status)
	}
}

func TestUpdateMerchantStatus_InvalidStatusesRejected(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	merch, _ := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:           uuid.New().String(),
		BusinessName: "Invalid Status Merchant",
	})

	invalidStatuses := []string{"REGISTERED", "SUSPENDED", "ACTIVE", "UNKNOWN", ""}
	for _, st := range invalidStatuses {
		_, err := svc.UpdateMerchantStatus(context.Background(), merch.ID, dto.UpdateMerchantStatusRequest{
			Status: st,
		})
		if err == nil {
			t.Errorf("expected status %q to be rejected, but it succeeded", st)
		}
	}
}

func TestGetMerchantByID(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	merchantID := uuid.New()
	merchant := &model.Merchant{
		ID:           merchantID,
		BusinessName: "Query Store",
		Status:       string(model.MerchantStatusPending),
	}
	_ = repo.Create(context.Background(), merchant)

	// GetByID
	res, err := svc.GetMerchantByID(context.Background(), merchant.ID)
	if err != nil {
		t.Fatalf("expected merchant by id, got %v", err)
	}
	if res.ID != merchantID {
		t.Errorf("expected merchant ID %s, got %s", merchantID, res.ID)
	}

	// Invalid inputs
	_, err = svc.GetMerchantByID(context.Background(), uuid.Nil)
	if err == nil {
		t.Fatal("expected error for nil merchant ID")
	}
}

func TestUpdateMerchant(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	merch, _ := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:           uuid.New().String(),
		BusinessName: "Original Name",
	})

	newName := "Updated Name"
	updated, err := svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
		BusinessName: newName,
		FirstName:    "UpdatedFirst",
		LastName:     "UpdatedLast",
	})
	if err != nil {
		t.Fatalf("UpdateMerchant failed: %v", err)
	}
	if updated.BusinessName != newName {
		t.Errorf("expected %s, got %s", newName, updated.BusinessName)
	}
	if updated.FirstName != "UpdatedFirst" || updated.LastName != "UpdatedLast" {
		t.Errorf("expected UpdatedFirst UpdatedLast, got %s %s", updated.FirstName, updated.LastName)
	}

	// Not found
	_, err = svc.UpdateMerchant(context.Background(), uuid.New(), dto.UpdateMerchantRequest{
		BusinessName: newName,
	})
	if err == nil {
		t.Fatal("expected error for non-existent merchant")
	}
}

func TestDeleteMerchant(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	merch, _ := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:           uuid.New().String(),
		BusinessName: "To Delete",
	})

	err := svc.DeleteMerchant(context.Background(), merch.ID)
	if err != nil {
		t.Fatalf("DeleteMerchant failed: %v", err)
	}

	_, err = svc.GetMerchantByID(context.Background(), merch.ID)
	if err == nil {
		t.Fatal("expected not found after deletion")
	}
}
