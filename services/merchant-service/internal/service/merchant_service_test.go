package service_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
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
	auditLogs     []*model.MerchantLifecycleAudit
	appeals       []*model.MerchantAppeal
	createErr     error
}

func newMockRepo() *mockMerchantRepository {
	return &mockMerchantRepository{
		merchantsByID: make(map[uuid.UUID]*model.Merchant),
		auditLogs:     make([]*model.MerchantLifecycleAudit, 0),
		appeals:       make([]*model.MerchantAppeal, 0),
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
	if !exists || merchant.DeletedAt != nil {
		return nil, appErrors.NotFound("merchant not found")
	}
	copied := *merchant
	return &copied, nil
}

func (m *mockMerchantRepository) List(ctx context.Context, limit, offset int, status string) ([]*model.Merchant, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []*model.Merchant
	for _, merch := range m.merchantsByID {
		if merch.DeletedAt != nil {
			continue
		}
		if status == "" || merch.Status == status {
			c := *merch
			list = append(list, &c)
		}
	}
	return list, len(list), nil
}

func (m *mockMerchantRepository) ListReactivated(ctx context.Context, limit, offset int) ([]*model.Merchant, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []*model.Merchant
	for _, merch := range m.merchantsByID {
		if merch.DeletedAt != nil {
			continue
		}
		if merch.Status == string(model.MerchantStatusApproved) || merch.Status == string(model.MerchantStatusActive) {
			for _, a := range m.auditLogs {
				if a.MerchantID == merch.ID {
					c := *merch
					list = append(list, &c)
					break
				}
			}
		}
	}
	return list, len(list), nil
}

func (m *mockMerchantRepository) Update(ctx context.Context, merchant *model.Merchant) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.merchantsByID[merchant.ID]
	if !exists || existing.DeletedAt != nil {
		return appErrors.NotFound("merchant not found")
	}

	// Persist mutable fields: business_name, first_name, last_name, business_phone, tax_id, and updated_at
	existing.BusinessName = merchant.BusinessName
	existing.FirstName = merchant.FirstName
	existing.LastName = merchant.LastName
	existing.BusinessPhone = merchant.BusinessPhone
	existing.PanCardNumber = merchant.PanCardNumber
	existing.UpdatedAt = time.Now().UTC()

	merchant.UpdatedAt = existing.UpdatedAt
	return nil
}

func (m *mockMerchantRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, rejectionReason string) (*model.Merchant, error) {
	merch, _, err := m.UpdateStatusWithAudit(ctx, id, model.MerchantStatus(status), rejectionReason, "ADMIN")
	return merch, err
}

func (m *mockMerchantRepository) UpdateStatusWithAudit(ctx context.Context, id uuid.UUID, newStatus model.MerchantStatus, reason string, updatedBy string) (*model.Merchant, model.MerchantStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	merch, exists := m.merchantsByID[id]
	if !exists || merch.DeletedAt != nil {
		return nil, "", appErrors.NotFound("merchant not found")
	}

	currentStatus := model.MerchantStatus(merch.Status)
	validator := model.NewStateTransitionValidator()
	if err := validator.Validate(currentStatus, newStatus); err != nil {
		return nil, currentStatus, err
	}

	merch.Status = string(newStatus)
	merch.RejectionReason = reason
	merch.UpdatedAt = time.Now().UTC()
	c := *merch
	m.merchantsByID[id] = &c
	return &c, currentStatus, nil
}

func (m *mockMerchantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	merch, exists := m.merchantsByID[id]
	if !exists || merch.DeletedAt != nil {
		return appErrors.NotFound("merchant not found")
	}
	now := time.Now().UTC()
	merch.DeletedAt = &now
	return nil
}

func (m *mockMerchantRepository) ExecuteLifecycleTransition(
	ctx context.Context,
	merchantID uuid.UUID,
	action model.LifecycleAction,
	reason string,
	adminID string,
	reqID string,
) (*model.Merchant, model.MerchantStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	merch, exists := m.merchantsByID[merchantID]
	if !exists || merch.DeletedAt != nil {
		return nil, "", appErrors.NotFound("merchant not found")
	}

	currentStatus := model.MerchantStatus(merch.Status)
	targetStatus, err := model.ValidateLifecycleTransition(action, currentStatus)
	if err != nil {
		audit := &model.MerchantLifecycleAudit{
			ID:             uuid.New(),
			MerchantID:     merchantID,
			AdminID:        adminID,
			Action:         action,
			PreviousStatus: string(currentStatus),
			NewStatus:      string(currentStatus),
			Reason:         reason,
			Status:         model.AuditStatusFailed,
			CreatedAt:      time.Now().UTC(),
		}
		m.auditLogs = append(m.auditLogs, audit)
		return nil, currentStatus, err
	}

	merch.Status = string(targetStatus)
	merch.Version++
	merch.UpdatedAt = time.Now().UTC()
	c := *merch
	m.merchantsByID[merchantID] = &c

	audit := &model.MerchantLifecycleAudit{
		ID:             uuid.New(),
		MerchantID:     merchantID,
		AdminID:        adminID,
		Action:         action,
		PreviousStatus: string(currentStatus),
		NewStatus:      string(targetStatus),
		Reason:         reason,
		Status:         model.AuditStatusSuccess,
		CreatedAt:      time.Now().UTC(),
	}
	m.auditLogs = append(m.auditLogs, audit)

	return &c, currentStatus, nil
}

func (m *mockMerchantRepository) RecordLifecycleAudit(ctx context.Context, audit *model.MerchantLifecycleAudit) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auditLogs = append(m.auditLogs, audit)
	return nil
}

func (m *mockMerchantRepository) CreateAppeal(ctx context.Context, appeal *model.MerchantAppeal) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if appeal.ID == uuid.Nil {
		appeal.ID = uuid.New()
	}
	now := time.Now().UTC()
	if appeal.CreatedAt.IsZero() {
		appeal.CreatedAt = now
	}
	if appeal.UpdatedAt.IsZero() {
		appeal.UpdatedAt = now
	}
	if appeal.Status == "" {
		appeal.Status = string(model.MerchantAppealStatusPending)
	}

	copied := *appeal
	m.appeals = append(m.appeals, &copied)
	return nil
}

func (m *mockMerchantRepository) GetAppealsByMerchantID(ctx context.Context, merchantID uuid.UUID) ([]*model.MerchantAppeal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []*model.MerchantAppeal
	for _, a := range m.appeals {
		if a.MerchantID == merchantID {
			copied := *a
			list = append(list, &copied)
		}
	}
	return list, nil
}


func TestCreateMerchant_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	email := "test@merchant.com"
	req := dto.CreateMerchantRequest{
		ID:            uuid.New().String(),
		FirstName:     "Jane",
		LastName:      "Smith",
		BusinessEmail: email,
	}

	merch, err := svc.CreateMerchant(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateMerchant failed: %v", err)
	}

	if merch.Status != string(model.MerchantStatusPending) {
		t.Errorf("expected initial status to be PENDING, got %s", merch.Status)
	}
	if merch.BusinessName != "" {
		t.Errorf("expected initial business name to be empty, got %s", merch.BusinessName)
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

	// Invalid ID format
	_, err := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:            "not-a-uuid",
		FirstName:     "Invalid",
		LastName:      "UUID",
		BusinessEmail: "test@example.com",
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
		ID:            uID,
		FirstName:     "First",
		LastName:      "User",
		BusinessEmail: "first@example.com",
	})
	if err != nil {
		t.Fatalf("first creation failed: %v", err)
	}

	_, err = svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:            uID,
		FirstName:     "Second",
		LastName:      "User",
		BusinessEmail: "second@example.com",
	})
	if err == nil {
		t.Fatal("expected conflict error for duplicate merchant ID")
	}
}

func TestGetMerchantByID(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	merchantID := uuid.New()
	merchant := &model.Merchant{
		ID:            merchantID,
		BusinessName:  "Query Store",
		FirstName:     "Alice",
		LastName:      "Wonder",
		BusinessEmail: "alice@store.com",
		BusinessPhone: "+1234567890",
		PanCardNumber: "TAX-999",
		Status:        string(model.MerchantStatusPending),
	}
	_ = repo.Create(context.Background(), merchant)

	// 1. GetByID Success
	res, err := svc.GetMerchantByID(context.Background(), merchant.ID)
	if err != nil {
		t.Fatalf("expected merchant by id, got %v", err)
	}
	if res.ID != merchantID {
		t.Errorf("expected merchant ID %s, got %s", merchantID, res.ID)
	}
	if res.BusinessName != "Query Store" {
		t.Errorf("expected Query Store, got %s", res.BusinessName)
	}
	if res.BusinessEmail != "alice@store.com" {
		t.Errorf("expected alice@store.com, got %s", res.BusinessEmail)
	}

	// 2. Invalid inputs - Nil UUID returns BadRequest
	_, err = svc.GetMerchantByID(context.Background(), uuid.Nil)
	if err == nil {
		t.Fatal("expected error for nil merchant ID")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BadRequest code, got %s", appErr.Code)
	}

	// 3. Non-existent UUID returns NotFound
	_, err = svc.GetMerchantByID(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for non-existent merchant ID")
	}
	appErr = appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeNotFound {
		t.Errorf("expected NotFound code, got %s", appErr.Code)
	}
}

func TestUpdateMerchant_Validations(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	merch, err := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:            uuid.New().String(),
		FirstName:     "Initial",
		LastName:      "Merchant",
		BusinessEmail: "initial@example.com",
	})
	if err != nil {
		t.Fatalf("failed to seed merchant: %v", err)
	}

	// --- BusinessName Validations ---
	t.Run("business_name validation", func(t *testing.T) {
		// Empty / whitespace
		for _, name := range []string{"", "   ", "\t\n"} {
			_, err := svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
				BusinessName:  name,
				BusinessPhone: "+1234567890",
				PanCardNumber:         "TAX-123",
			})
			if err == nil {
				t.Errorf("expected error for empty business_name %q", name)
			}
		}

		// Too short (< 2 characters)
		_, err := svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
			BusinessName:  "A",
			BusinessPhone: "+1234567890",
			PanCardNumber:         "TAX-123",
		})
		if err == nil {
			t.Error("expected error for business_name < 2 chars")
		}

		// Too long (> 100 characters)
		longName := strings.Repeat("A", 101)
		_, err = svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
			BusinessName:  longName,
			BusinessPhone: "+1234567890",
			PanCardNumber:         "TAX-123",
		})
		if err == nil {
			t.Error("expected error for business_name > 100 chars")
		}

		// Valid bounds: 2 chars and 100 chars
		for _, validName := range []string{"AB", strings.Repeat("B", 100), "Valid Company LLC"} {
			updated, err := svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
				BusinessName:  validName,
				BusinessPhone: "+1234567890",
				PanCardNumber:         "TAX-123",
			})
			if err != nil {
				t.Errorf("expected valid business_name %q to succeed, got %v", validName, err)
			}
			if updated.BusinessName != validName {
				t.Errorf("expected %s, got %s", validName, updated.BusinessName)
			}
		}
	})

	// --- BusinessPhone Validations ---
	t.Run("business_phone validation", func(t *testing.T) {
		invalidPhones := []string{
			"",
			"   ",
			"phone123",
			"123",                  // too short (< 7 digits)
			"+0123456789",          // country code cannot start with 0
			"++1234567890",         // double plus
			"12345678901234567890", // too long (> 15 digits)
			"abc-def-ghij",
		}
		for _, phone := range invalidPhones {
			_, err := svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
				BusinessName:  "Valid Name",
				BusinessPhone: phone,
				PanCardNumber:         "TAX-123",
			})
			if err == nil {
				t.Errorf("expected error for invalid phone %q, but succeeded", phone)
			}
		}

		validPhones := []string{
			"+1234567890",
			"+442071838750",
			"+4915123456789",
			"+919876543210",
			"1234567890",
		}
		for _, phone := range validPhones {
			updated, err := svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
				BusinessName:  "Valid Name",
				BusinessPhone: phone,
				PanCardNumber:         "TAX-123",
			})
			if err != nil {
				t.Errorf("expected phone %q to succeed, got: %v", phone, err)
			}
			if updated.BusinessPhone != phone {
				t.Errorf("expected phone %s, got %s", phone, updated.BusinessPhone)
			}
		}
	})

	// --- TaxID Validations ---
	t.Run("tax_id validation", func(t *testing.T) {
		invalidTaxIDs := []string{
			"",
			"   ",
			"12",                    // too short (< 3 chars)
			strings.Repeat("X", 51), // too long (> 50 chars)
			"TAX@123",               // invalid special characters
			"TAX$#*",
		}
		for _, tid := range invalidTaxIDs {
			_, err := svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
				BusinessName:  "Valid Name",
				BusinessPhone: "+1234567890",
				PanCardNumber:         tid,
			})
			if err == nil {
				t.Errorf("expected error for invalid tax_id %q, but succeeded", tid)
			}
		}

		validTaxIDs := []string{
			"TAX-12345",
			"12-3456789",
			"DE123456789",
			"GB-999-888",
			"12345678",
		}
		for _, tid := range validTaxIDs {
			updated, err := svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
				BusinessName:  "Valid Name",
				BusinessPhone: "+1234567890",
				PanCardNumber:         tid,
			})
			if err != nil {
				t.Errorf("expected tax_id %q to succeed, got: %v", tid, err)
			}
			if updated.PanCardNumber != tid {
				t.Errorf("expected tax_id %s, got %s", tid, updated.PanCardNumber)
			}
		}
	})

	// --- ID & Not Found Validations ---
	t.Run("id validation and not found", func(t *testing.T) {
		// Nil UUID returns BadRequest
		_, err := svc.UpdateMerchant(context.Background(), uuid.Nil, dto.UpdateMerchantRequest{
			BusinessName:  "Valid Name",
			BusinessPhone: "+1234567890",
			PanCardNumber:         "TAX-123",
		})
		if err == nil {
			t.Fatal("expected error for nil UUID on update")
		}

		// Non-existent merchant returns NotFound
		_, err = svc.UpdateMerchant(context.Background(), uuid.New(), dto.UpdateMerchantRequest{
			BusinessName:  "Valid Name",
			BusinessPhone: "+1234567890",
			PanCardNumber:         "TAX-123",
		})
		if err == nil {
			t.Fatal("expected NotFound error for non-existent merchant")
		}
		appErr := appErrors.AsAppError(err)
		if appErr.Code != appErrors.CodeNotFound {
			t.Errorf("expected NotFound code, got %s", appErr.Code)
		}
	})
}

func TestUpdateMerchant_ImmutabilityAndPersistence(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	merchantID := uuid.New()
	initialCreatedAt := time.Now().UTC().Add(-2 * time.Hour)
	initialUpdatedAt := time.Now().UTC().Add(-2 * time.Hour)

	seeded := &model.Merchant{
		ID:              merchantID,
		BusinessName:    "Original Store",
		FirstName:       "OriginalFirst",
		LastName:        "OriginalLast",
		BusinessEmail:   "original@example.com",
		BusinessPhone:   "+1111111111",
		PanCardNumber:   "TAX-ORIGINAL",
		Status:          string(model.MerchantStatusApproved),
		RejectionReason: "Original Reason",
		CreatedAt:       initialCreatedAt,
		UpdatedAt:       initialUpdatedAt,
	}
	_ = repo.Create(context.Background(), seeded)
	// Force original timestamps
	seeded.CreatedAt = initialCreatedAt
	seeded.UpdatedAt = initialUpdatedAt
	repo.merchantsByID[merchantID] = seeded

	// Update mutable fields: business_name, business_phone, tax_id
	time.Sleep(10 * time.Millisecond) // ensure timestamp ticks
	updated, err := svc.UpdateMerchant(context.Background(), merchantID, dto.UpdateMerchantRequest{
		BusinessName:  "Updated Super Store",
		BusinessPhone: "+19999999999",
		PanCardNumber:         "TAX-UPDATED-999",
	})
	if err != nil {
		t.Fatalf("UpdateMerchant failed: %v", err)
	}

	// 1. Verify mutable fields were updated
	if updated.BusinessName != "Updated Super Store" {
		t.Errorf("expected updated business_name, got %s", updated.BusinessName)
	}
	if updated.BusinessPhone != "+19999999999" {
		t.Errorf("expected updated business_phone, got %s", updated.BusinessPhone)
	}
	if updated.PanCardNumber != "TAX-UPDATED-999" {
		t.Errorf("expected updated tax_id, got %s", updated.PanCardNumber)
	}

	// 2. Verify updated_at was automatically refreshed
	if !updated.UpdatedAt.After(initialUpdatedAt) {
		t.Errorf("expected UpdatedAt to be after initial (%v), got %v", initialUpdatedAt, updated.UpdatedAt)
	}

	// 3. Verify non-editable / immutable fields remain strictly unchanged
	if updated.ID != merchantID {
		t.Errorf("ID must not change: expected %s, got %s", merchantID, updated.ID)
	}
	if updated.FirstName != "OriginalFirst" {
		t.Errorf("FirstName must remain unchanged: expected OriginalFirst, got %s", updated.FirstName)
	}
	if updated.LastName != "OriginalLast" {
		t.Errorf("LastName must remain unchanged: expected OriginalLast, got %s", updated.LastName)
	}
	if updated.BusinessEmail != "original@example.com" {
		t.Errorf("BusinessEmail must remain unchanged: expected original@example.com, got %s", updated.BusinessEmail)
	}
	if updated.Status != string(model.MerchantStatusApproved) {
		t.Errorf("Status must remain unchanged: expected APPROVED, got %s", updated.Status)
	}
	if updated.RejectionReason != "Original Reason" {
		t.Errorf("RejectionReason must remain unchanged: expected 'Original Reason', got %s", updated.RejectionReason)
	}
	if !updated.CreatedAt.Equal(initialCreatedAt) {
		t.Errorf("CreatedAt must remain unchanged: expected %v, got %v", initialCreatedAt, updated.CreatedAt)
	}

	// 4. Persistence check: Fetch directly from repo and verify persistence
	persisted, err := repo.GetByID(context.Background(), merchantID)
	if err != nil {
		t.Fatalf("failed to reload persisted merchant: %v", err)
	}
	if persisted.BusinessName != "Updated Super Store" {
		t.Errorf("persisted business_name mismatch: %s", persisted.BusinessName)
	}
	if persisted.BusinessPhone != "+19999999999" {
		t.Errorf("persisted business_phone mismatch: %s", persisted.BusinessPhone)
	}
	if persisted.PanCardNumber != "TAX-UPDATED-999" {
		t.Errorf("persisted tax_id mismatch: %s", persisted.PanCardNumber)
	}
	if persisted.FirstName != "OriginalFirst" || persisted.LastName != "OriginalLast" {
		t.Errorf("persisted names must remain unchanged: %s %s", persisted.FirstName, persisted.LastName)
	}
	if persisted.Status != string(model.MerchantStatusApproved) {
		t.Errorf("persisted status must remain unchanged: %s", persisted.Status)
	}
}

func TestUpdateMerchant_UpdateFirstAndLastName(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	merchantID := uuid.New()
	seeded := &model.Merchant{
		ID:            merchantID,
		BusinessName:  "Test Store",
		FirstName:     "John",
		LastName:      "Doe",
		BusinessEmail: "john.doe@example.com",
		BusinessPhone: "+1234567890",
		PanCardNumber: "TAX-12345",
		Status:        string(model.MerchantStatusApproved),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	_ = repo.Create(context.Background(), seeded)

	updated, err := svc.UpdateMerchant(context.Background(), merchantID, dto.UpdateMerchantRequest{
		BusinessName:  "Updated Store",
		FirstName:     "Jane",
		LastName:      "Smith",
		BusinessPhone: "+1987654321",
		PanCardNumber: "TAX-99999",
	})
	if err != nil {
		t.Fatalf("UpdateMerchant failed: %v", err)
	}

	if updated.FirstName != "Jane" {
		t.Errorf("expected updated FirstName Jane, got %s", updated.FirstName)
	}
	if updated.LastName != "Smith" {
		t.Errorf("expected updated LastName Smith, got %s", updated.LastName)
	}

	persisted, err := repo.GetByID(context.Background(), merchantID)
	if err != nil {
		t.Fatalf("failed to reload persisted merchant: %v", err)
	}
	if persisted.FirstName != "Jane" || persisted.LastName != "Smith" {
		t.Errorf("expected persisted names Jane Smith, got %s %s", persisted.FirstName, persisted.LastName)
	}
}

func TestUpdateMerchantStatus(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	merch, err := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:            uuid.New().String(),
		FirstName:     "Status",
		LastName:      "Merchant",
		BusinessEmail: "status@example.com",
	})
	if err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	// 1. Update to APPROVED (PENDING -> APPROVED: Valid)
	updated, prevStatus, err := svc.UpdateMerchantStatus(context.Background(), merch.ID, dto.UpdateMerchantStatusRequest{
		Status: "APPROVED",
	})
	if err != nil {
		t.Fatalf("expected APPROVED to succeed, got %v", err)
	}
	if updated.Status != "APPROVED" {
		t.Errorf("expected APPROVED, got %s", updated.Status)
	}
	if prevStatus != "PENDING" {
		t.Errorf("expected prevStatus PENDING, got %s", prevStatus)
	}

	// 2. Update to SUSPENDED (APPROVED -> SUSPENDED: Valid)
	suspendReason := "Policy violation"
	updated, prevStatus, err = svc.UpdateMerchantStatus(context.Background(), merch.ID, dto.UpdateMerchantStatusRequest{
		Status:          "SUSPENDED",
		RejectionReason: suspendReason,
	})
	if err != nil {
		t.Fatalf("expected SUSPENDED to succeed, got %v", err)
	}
	if updated.Status != "SUSPENDED" {
		t.Errorf("expected SUSPENDED, got %s", updated.Status)
	}
	if prevStatus != "APPROVED" {
		t.Errorf("expected prevStatus APPROVED, got %s", prevStatus)
	}

	// 3. Test PENDING -> REJECTED with a fresh merchant
	merch2, err := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:            uuid.New().String(),
		FirstName:     "Rejected",
		LastName:      "Merchant",
		BusinessEmail: "rejected@example.com",
	})
	if err != nil {
		t.Fatalf("failed to create merchant2: %v", err)
	}

	reason := "Incomplete documentation"
	updated2, prevStatus2, err := svc.UpdateMerchantStatus(context.Background(), merch2.ID, dto.UpdateMerchantStatusRequest{
		Status:          "REJECTED",
		RejectionReason: reason,
	})
	if err != nil {
		t.Fatalf("expected REJECTED to succeed, got %v", err)
	}
	if updated2.Status != "REJECTED" {
		t.Errorf("expected REJECTED, got %s", updated2.Status)
	}
	if prevStatus2 != "PENDING" {
		t.Errorf("expected prevStatus2 PENDING, got %s", prevStatus2)
	}
	if updated2.RejectionReason != reason {
		t.Errorf("expected reason %s, got %s", reason, updated2.RejectionReason)
	}

	// 4. Invalid transition: REJECTED -> APPROVED must fail
	_, _, err = svc.UpdateMerchantStatus(context.Background(), merch2.ID, dto.UpdateMerchantStatusRequest{
		Status: "APPROVED",
	})
	if err == nil {
		t.Error("expected error for invalid transition REJECTED -> APPROVED")
	}

	// 5. Invalid status rejected
	_, _, err = svc.UpdateMerchantStatus(context.Background(), merch.ID, dto.UpdateMerchantStatusRequest{
		Status: "INVALID_STATUS",
	})
	if err == nil {
		t.Error("expected error for invalid status")
	}

	// 6. REJECTED without rejectionReason must fail
	_, _, err = svc.UpdateMerchantStatus(context.Background(), merch2.ID, dto.UpdateMerchantStatusRequest{
		Status:          "REJECTED",
		RejectionReason: "",
	})
	if err == nil {
		t.Error("expected error when rejecting without rejectionReason")
	}

	// 7. SUSPENDED without rejectionReason must fail
	_, _, err = svc.UpdateMerchantStatus(context.Background(), merch.ID, dto.UpdateMerchantStatusRequest{
		Status:          "SUSPENDED",
		RejectionReason: "",
	})
	if err == nil {
		t.Error("expected error when suspending without rejectionReason")
	}
}

func TestDeleteMerchant(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	merch, _ := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:            uuid.New().String(),
		FirstName:     "To",
		LastName:      "Delete",
		BusinessEmail: "todelete@example.com",
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

func TestMerchantService_Logging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, logger)

	// Create
	merchID := uuid.New().String()
	merch, err := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:            merchID,
		FirstName:     "Log",
		LastName:      "Tester",
		BusinessEmail: "log@example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error creating merchant: %v", err)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "Creating merchant profile") {
		t.Errorf("expected log to contain 'Creating merchant profile', got %s", logOutput)
	}
	if !strings.Contains(logOutput, "Merchant profile created successfully") {
		t.Errorf("expected log to contain 'Merchant profile created successfully', got %s", logOutput)
	}

	// Update
	buf.Reset()
	_, err = svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
		BusinessName:  "Logged Business",
		BusinessPhone: "+1234567890",
		PanCardNumber: "TAX-LOG-1",
	})
	if err != nil {
		t.Fatalf("unexpected error updating merchant: %v", err)
	}
	logOutput = buf.String()
	if !strings.Contains(logOutput, "Updating merchant business details") {
		t.Errorf("expected log to contain 'Updating merchant business details', got %s", logOutput)
	}
	if !strings.Contains(logOutput, "Merchant business details updated successfully") {
		t.Errorf("expected log to contain 'Merchant business details updated successfully', got %s", logOutput)
	}

	// Get
	buf.Reset()
	_, err = svc.GetMerchantByID(context.Background(), merch.ID)
	if err != nil {
		t.Fatalf("unexpected error getting merchant: %v", err)
	}
	logOutput = buf.String()
	if !strings.Contains(logOutput, "Retrieving merchant profile") {
		t.Errorf("expected log to contain 'Retrieving merchant profile', got %s", logOutput)
	}

	// Delete
	buf.Reset()
	err = svc.DeleteMerchant(context.Background(), merch.ID)
	if err != nil {
		t.Fatalf("unexpected error deleting merchant: %v", err)
	}
	logOutput = buf.String()
	if !strings.Contains(logOutput, "Deleting merchant profile") {
		t.Errorf("expected log to contain 'Deleting merchant profile', got %s", logOutput)
	}
}

func TestDeleteMerchant_SoftDelete(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	created, err := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:            uuid.New().String(),
		FirstName:     "Soft",
		LastName:      "Delete",
		BusinessEmail: "softdelete@test.com",
	})
	if err != nil {
		t.Fatalf("unexpected error creating merchant: %v", err)
	}

	// First delete should succeed (soft delete)
	err = svc.DeleteMerchant(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected successful soft delete, got: %v", err)
	}

	// Subsequent GetByID should return NotFound
	_, err = svc.GetMerchantByID(context.Background(), created.ID)
	if err == nil {
		t.Fatalf("expected not found error after soft delete, got nil")
	}

	// Subsequent UpdateMerchant should return NotFound
	_, err = svc.UpdateMerchant(context.Background(), created.ID, dto.UpdateMerchantRequest{
		BusinessName:  "Updated Name",
		BusinessPhone: "+1234567890",
		PanCardNumber:         "TAX-12345",
	})
	if err == nil {
		t.Fatalf("expected not found error when updating soft-deleted merchant, got nil")
	}

	// Subsequent DeleteMerchant should return NotFound
	err = svc.DeleteMerchant(context.Background(), created.ID)
	if err == nil {
		t.Fatalf("expected not found error when deleting already soft-deleted merchant, got nil")
	}
}

func TestMerchantLifecycle_ServiceSuccessAndTransitions(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	created, err := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:            uuid.New().String(),
		BusinessEmail: "lifecycle@merchant.com",
		FirstName:     "Life",
		LastName:      "Cycle",
	})
	if err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	adminID := "admin-user-1"
	reqID := "req-trace-1"

	// 1. Activate from PENDING -> ACTIVE
	activated, prev, err := svc.ActivateMerchant(context.Background(), created.ID, "Approved KYC", adminID, reqID)
	if err != nil {
		t.Fatalf("ActivateMerchant failed: %v", err)
	}
	if prev != model.MerchantStatusPending {
		t.Errorf("expected previous status PENDING, got %s", prev)
	}
	if activated.Status != string(model.MerchantStatusActive) {
		t.Errorf("expected new status ACTIVE, got %s", activated.Status)
	}

	// 2. Suspend from ACTIVE -> SUSPENDED
	suspended, prev, err := svc.SuspendMerchant(context.Background(), created.ID, "Policy violation", adminID, reqID)
	if err != nil {
		t.Fatalf("SuspendMerchant failed: %v", err)
	}
	if prev != model.MerchantStatusActive {
		t.Errorf("expected previous status ACTIVE, got %s", prev)
	}
	if suspended.Status != string(model.MerchantStatusSuspended) {
		t.Errorf("expected new status SUSPENDED, got %s", suspended.Status)
	}

	// 3. Reactivate from SUSPENDED -> ACTIVE
	reactivated, prev, err := svc.ReactivateMerchant(context.Background(), created.ID, "Resolved", adminID, reqID)
	if err != nil {
		t.Fatalf("ReactivateMerchant failed: %v", err)
	}
	if prev != model.MerchantStatusSuspended {
		t.Errorf("expected previous status SUSPENDED, got %s", prev)
	}
	if reactivated.Status != string(model.MerchantStatusActive) {
		t.Errorf("expected new status ACTIVE, got %s", reactivated.Status)
	}
}

func TestMerchantLifecycle_ServiceInvalidTransitionsAndConflicts(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	created, _ := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:            uuid.New().String(),
		BusinessEmail: "conflict@merchant.com",
		FirstName:     "Conflict",
		LastName:      "User",
	})

	// Reactivating a PENDING merchant should fail with 409 Conflict
	_, _, err := svc.ReactivateMerchant(context.Background(), created.ID, "Try reactivate", "admin-1", "trace-1")
	if err == nil {
		t.Fatalf("expected error reactivating PENDING merchant, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeConflict {
		t.Fatalf("expected CONFLICT code, got %v", err)
	}

	// Activating an already ACTIVE merchant should fail with 409 Conflict
	_, _, err = svc.ActivateMerchant(context.Background(), created.ID, "Activate 1", "admin-1", "trace-1")
	if err != nil {
		t.Fatalf("first activate failed: %v", err)
	}
	_, _, err = svc.ActivateMerchant(context.Background(), created.ID, "Activate 2", "admin-1", "trace-1")
	if err == nil {
		t.Fatalf("expected conflict on second activate, got nil")
	}
	appErr = appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeConflict {
		t.Fatalf("expected CONFLICT code, got %v", err)
	}
}

func TestMerchantLifecycle_ValidationErrors(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	_, _, err := svc.ActivateMerchant(context.Background(), uuid.Nil, "reason", "admin-1", "req-1")
	if err == nil {
		t.Fatalf("expected error on nil uuid")
	}

	_, _, err = svc.SuspendMerchant(context.Background(), uuid.Nil, "reason", "admin-1", "req-1")
	if err == nil {
		t.Fatalf("expected error on nil uuid")
	}

	_, _, err = svc.ReactivateMerchant(context.Background(), uuid.Nil, "reason", "admin-1", "req-1")
	if err == nil {
		t.Fatalf("expected error on nil uuid")
	}
}

func TestMerchantAppeals(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	created, err := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:            uuid.New().String(),
		BusinessEmail: "appeal@merchant.com",
		FirstName:     "Appeal",
		LastName:      "Merchant",
	})
	if err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	// 1. Appeal on PENDING merchant fails with Conflict (must be SUSPENDED)
	_, err = svc.CreateAppeal(context.Background(), created.ID, "Unfair suspension")
	if err == nil {
		t.Fatalf("expected conflict error when appealing non-suspended merchant, got nil")
	}

	// 2. Activate then Suspend merchant
	_, _, err = svc.ActivateMerchant(context.Background(), created.ID, "Activating", "admin-1", "req-1")
	if err != nil {
		t.Fatalf("activate failed: %v", err)
	}
	_, _, err = svc.SuspendMerchant(context.Background(), created.ID, "Violated policy", "admin-1", "req-2")
	if err != nil {
		t.Fatalf("suspend failed: %v", err)
	}

	// 3. Appeal with empty reason fails with BadRequest
	_, err = svc.CreateAppeal(context.Background(), created.ID, "   ")
	if err == nil {
		t.Fatalf("expected error for empty reason, got nil")
	}

	// 4. Appeal with nil UUID fails
	_, err = svc.CreateAppeal(context.Background(), uuid.Nil, "Valid reason")
	if err == nil {
		t.Fatalf("expected error for nil merchantID, got nil")
	}

	// 5. Successful appeal
	appeal, err := svc.CreateAppeal(context.Background(), created.ID, "Policy issue resolved")
	if err != nil {
		t.Fatalf("create appeal failed: %v", err)
	}
	if appeal.ID == uuid.Nil {
		t.Errorf("expected generated appeal ID, got nil")
	}
	if appeal.Status != string(model.MerchantAppealStatusPending) {
		t.Errorf("expected pending status, got %s", appeal.Status)
	}

	// Verify merchant status changed to PENDING
	reloaded, err := svc.GetMerchantByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get merchant failed: %v", err)
	}
	if reloaded.Status != string(model.MerchantStatusPending) {
		t.Errorf("expected merchant status PENDING after appeal, got %s", reloaded.Status)
	}

	// 6. Get appeals
	appeals, err := svc.GetAppeals(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get appeals failed: %v", err)
	}
	if len(appeals) != 1 {
		t.Fatalf("expected 1 appeal, got %d", len(appeals))
	}
	if appeals[0].Reason != "Policy issue resolved" {
		t.Errorf("expected reason 'Policy issue resolved', got %s", appeals[0].Reason)
	}

	// 7. Get appeals for nil UUID
	_, err = svc.GetAppeals(context.Background(), uuid.Nil)
	if err == nil {
		t.Fatalf("expected error for nil merchantID in GetAppeals")
	}
}

func TestListReactivatedMerchants(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewMerchantService(repo, nil)

	created, err := svc.CreateMerchant(context.Background(), dto.CreateMerchantRequest{
		ID:            uuid.New().String(),
		BusinessEmail: "reactivated@merchant.com",
		FirstName:     "Reactivated",
		LastName:      "User",
	})
	if err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	// Activate -> Suspend -> Reactivate
	_, _, err = svc.ActivateMerchant(context.Background(), created.ID, "Initial activation", "admin-1", "req-1")
	if err != nil {
		t.Fatalf("activate failed: %v", err)
	}
	_, _, err = svc.SuspendMerchant(context.Background(), created.ID, "Suspended for review", "admin-1", "req-2")
	if err != nil {
		t.Fatalf("suspend failed: %v", err)
	}
	_, _, err = svc.ReactivateMerchant(context.Background(), created.ID, "Reactivated after appeal", "admin-1", "req-3")
	if err != nil {
		t.Fatalf("reactivate failed: %v", err)
	}

	// Query reactivated merchants
	list, total, err := svc.ListReactivatedMerchants(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("list reactivated failed: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("expected 1 reactivated merchant, got total=%d len=%d", total, len(list))
	}
	if list[0].ID != created.ID {
		t.Errorf("expected merchant ID %v, got %v", created.ID, list[0].ID)
	}
}


