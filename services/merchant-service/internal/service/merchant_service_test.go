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

func (m *mockMerchantRepository) Update(ctx context.Context, merchant *model.Merchant) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.merchantsByID[merchant.ID]
	if !exists || existing.DeletedAt != nil {
		return appErrors.NotFound("merchant not found")
	}

	// Persist only mutable fields: business_name, business_phone, tax_id, and updated_at
	existing.BusinessName = merchant.BusinessName
	existing.BusinessPhone = merchant.BusinessPhone
	existing.TaxID = merchant.TaxID
	existing.UpdatedAt = time.Now().UTC()

	merchant.UpdatedAt = existing.UpdatedAt
	return nil
}

func (m *mockMerchantRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, rejectionReason string) (*model.Merchant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	merch, exists := m.merchantsByID[id]
	if !exists || merch.DeletedAt != nil {
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

	merch, exists := m.merchantsByID[id]
	if !exists || merch.DeletedAt != nil {
		return appErrors.NotFound("merchant not found")
	}
	now := time.Now().UTC()
	merch.DeletedAt = &now
	return nil
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
		TaxID:         "TAX-999",
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
				TaxID:         "TAX-123",
			})
			if err == nil {
				t.Errorf("expected error for empty business_name %q", name)
			}
		}

		// Too short (< 2 characters)
		_, err := svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
			BusinessName:  "A",
			BusinessPhone: "+1234567890",
			TaxID:         "TAX-123",
		})
		if err == nil {
			t.Error("expected error for business_name < 2 chars")
		}

		// Too long (> 100 characters)
		longName := strings.Repeat("A", 101)
		_, err = svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
			BusinessName:  longName,
			BusinessPhone: "+1234567890",
			TaxID:         "TAX-123",
		})
		if err == nil {
			t.Error("expected error for business_name > 100 chars")
		}

		// Valid bounds: 2 chars and 100 chars
		for _, validName := range []string{"AB", strings.Repeat("B", 100), "Valid Company LLC"} {
			updated, err := svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
				BusinessName:  validName,
				BusinessPhone: "+1234567890",
				TaxID:         "TAX-123",
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
			"123",          // too short (< 7 digits)
			"+0123456789",  // country code cannot start with 0
			"++1234567890", // double plus
			"12345678901234567890", // too long (> 15 digits)
			"abc-def-ghij",
		}
		for _, phone := range invalidPhones {
			_, err := svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
				BusinessName:  "Valid Name",
				BusinessPhone: phone,
				TaxID:         "TAX-123",
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
				TaxID:         "TAX-123",
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
			"12",                       // too short (< 3 chars)
			strings.Repeat("X", 51),    // too long (> 50 chars)
			"TAX@123",                  // invalid special characters
			"TAX$#*",
		}
		for _, tid := range invalidTaxIDs {
			_, err := svc.UpdateMerchant(context.Background(), merch.ID, dto.UpdateMerchantRequest{
				BusinessName:  "Valid Name",
				BusinessPhone: "+1234567890",
				TaxID:         tid,
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
				TaxID:         tid,
			})
			if err != nil {
				t.Errorf("expected tax_id %q to succeed, got: %v", tid, err)
			}
			if updated.TaxID != tid {
				t.Errorf("expected tax_id %s, got %s", tid, updated.TaxID)
			}
		}
	})

	// --- ID & Not Found Validations ---
	t.Run("id validation and not found", func(t *testing.T) {
		// Nil UUID returns BadRequest
		_, err := svc.UpdateMerchant(context.Background(), uuid.Nil, dto.UpdateMerchantRequest{
			BusinessName:  "Valid Name",
			BusinessPhone: "+1234567890",
			TaxID:         "TAX-123",
		})
		if err == nil {
			t.Fatal("expected error for nil UUID on update")
		}

		// Non-existent merchant returns NotFound
		_, err = svc.UpdateMerchant(context.Background(), uuid.New(), dto.UpdateMerchantRequest{
			BusinessName:  "Valid Name",
			BusinessPhone: "+1234567890",
			TaxID:         "TAX-123",
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
		TaxID:           "TAX-ORIGINAL",
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
		TaxID:         "TAX-UPDATED-999",
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
	if updated.TaxID != "TAX-UPDATED-999" {
		t.Errorf("expected updated tax_id, got %s", updated.TaxID)
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
	if persisted.TaxID != "TAX-UPDATED-999" {
		t.Errorf("persisted tax_id mismatch: %s", persisted.TaxID)
	}
	if persisted.FirstName != "OriginalFirst" || persisted.LastName != "OriginalLast" {
		t.Errorf("persisted names must remain unchanged: %s %s", persisted.FirstName, persisted.LastName)
	}
	if persisted.Status != string(model.MerchantStatusApproved) {
		t.Errorf("persisted status must remain unchanged: %s", persisted.Status)
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

	// Update to APPROVED
	updated, err := svc.UpdateMerchantStatus(context.Background(), merch.ID, dto.UpdateMerchantStatusRequest{
		Status: "APPROVED",
	})
	if err != nil {
		t.Fatalf("expected APPROVED to succeed, got %v", err)
	}
	if updated.Status != "APPROVED" {
		t.Errorf("expected APPROVED, got %s", updated.Status)
	}

	// Update to REJECTED with reason
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

	// Invalid status rejected
	_, err = svc.UpdateMerchantStatus(context.Background(), merch.ID, dto.UpdateMerchantStatusRequest{
		Status: "INVALID_STATUS",
	})
	if err == nil {
		t.Error("expected error for invalid status")
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
		TaxID:         "TAX-LOG-1",
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
		TaxID:         "TAX-12345",
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
