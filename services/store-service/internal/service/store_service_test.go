package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/service"
)

type mockStoreRepository struct {
	stores map[string]*model.Store
	slugs  map[string]string
}

func newMockRepo() *mockStoreRepository {
	return &mockStoreRepository{
		stores: make(map[string]*model.Store),
		slugs:  make(map[string]string),
	}
}

func (m *mockStoreRepository) Create(ctx context.Context, store *model.Store) error {
	if store.ID == "" {
		store.ID = "store-" + store.Slug
	}
	store.CreatedAt = time.Now()
	store.UpdatedAt = time.Now()
	m.stores[store.ID] = store
	m.slugs[store.Slug] = store.ID
	return nil
}

func (m *mockStoreRepository) GetByID(ctx context.Context, id string) (*model.Store, error) {
	s, exists := m.stores[id]
	if !exists {
		return nil, appErrors.NotFound("store not found")
	}
	cp := *s
	return &cp, nil
}

func (m *mockStoreRepository) GetByMerchantID(ctx context.Context, merchantID string) (*model.Store, error) {
	for _, s := range m.stores {
		if s.MerchantID == merchantID {
			cp := *s
			return &cp, nil
		}
	}
	return nil, appErrors.NotFound("store not found for this merchant")
}

func (m *mockStoreRepository) GetBySlug(ctx context.Context, slug string) (*model.Store, error) {
	id, exists := m.slugs[slug]
	if !exists {
		return nil, appErrors.NotFound("store not found")
	}
	s := m.stores[id]
	cp := *s
	return &cp, nil
}

func (m *mockStoreRepository) List(ctx context.Context, merchantID string, limit, offset int) ([]*model.Store, int, error) {
	result := make([]*model.Store, 0)
	for _, s := range m.stores {
		if merchantID == "" || s.MerchantID == merchantID {
			cp := *s
			result = append(result, &cp)
		}
	}
	return result, len(result), nil
}

func (m *mockStoreRepository) Update(ctx context.Context, store *model.Store) error {
	existing, exists := m.stores[store.ID]
	if !exists {
		return appErrors.NotFound("store not found")
	}
	if existing.Slug != store.Slug {
		delete(m.slugs, existing.Slug)
		m.slugs[store.Slug] = store.ID
	}
	store.UpdatedAt = time.Now()
	m.stores[store.ID] = store
	return nil
}

func (m *mockStoreRepository) IsSlugAvailable(ctx context.Context, slug string, excludeID string) (bool, error) {
	existingID, exists := m.slugs[slug]
	if !exists {
		return true, nil
	}
	if excludeID != "" && existingID == excludeID {
		return true, nil
	}
	return false, nil
}

func (m *mockStoreRepository) UpdateStatus(ctx context.Context, id string, expectedStatus, newStatus string, rejectionReason *string) (*model.Store, error) {
	existing, exists := m.stores[id]
	if !exists {
		return nil, appErrors.NotFound("store not found")
	}
	if existing.ApprovalStatus != expectedStatus {
		return nil, appErrors.UnprocessableEntity("invalid state transition")
	}
	existing.ApprovalStatus = newStatus
	existing.RejectionReason = rejectionReason
	existing.UpdatedAt = time.Now()
	m.stores[id] = existing
	cp := *existing
	return &cp, nil
}

func TestCreateStore_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	authCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-123",
		Role:   auth.RoleMerchant,
	})

	bankDetails := `{"account_number":"123456789","bank_name":"Test Bank"}`
	req := dto.CreateStoreRequest{
		Name:               "Happy Shop",
		BusinessEmail:      "happyshop@example.com",
		BusinessPhone:      "+0 1234567890",
		Description:        "At Happy Shop, we believe shopping should be simple, smart, and satisfying.",
		LogoURL:            "https://example.com/logo.png",
		Address:            "3rd Floor, Happy Shop, New Building, 123 street, c sector, NY, US",
		BankAccountDetails: &bankDetails,
	}

	store, err := svc.CreateStore(authCtx, "", req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if store.MerchantID != "merchant-123" {
		t.Errorf("expected merchant_id merchant-123, got %s", store.MerchantID)
	}
	if store.Name != "Happy Shop" {
		t.Errorf("expected name Happy Shop, got %s", store.Name)
	}
	if store.Slug != "happy-shop" {
		t.Errorf("expected slug happy-shop, got %s", store.Slug)
	}
	if store.BusinessEmail != "happyshop@example.com" {
		t.Errorf("expected business_email happyshop@example.com, got %s", store.BusinessEmail)
	}
	if store.BusinessPhone != "+0 1234567890" {
		t.Errorf("expected business_phone +0 1234567890, got %s", store.BusinessPhone)
	}
	if store.ApprovalStatus != model.StoreStatusDraft {
		t.Errorf("expected status %s, got %s", model.StoreStatusDraft, store.ApprovalStatus)
	}
	if store.IsVacationMode != false {
		t.Errorf("expected is_vacation_mode false, got %v", store.IsVacationMode)
	}
	if store.Address != "3rd Floor, Happy Shop, New Building, 123 street, c sector, NY, US" {
		t.Errorf("unexpected address: %s", store.Address)
	}
}

func TestCreateStore_DeriveMerchantFromAuthContext(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	// Auth context has authenticated merchant ID "auth-merchant"
	authCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "auth-merchant",
		Role:   auth.RoleMerchant,
	})

	// Client sends spoofed "attacker-merchant" in body
	req := dto.CreateStoreRequest{
		MerchantID: "attacker-merchant",
		Name:       "Secure Merchant Store",
	}

	store, err := svc.CreateStore(authCtx, "", req)
	if err != nil {
		t.Fatalf("expected create success, got %v", err)
	}

	// Must derive from auth context rather than trusted from client body
	if store.MerchantID != "auth-merchant" {
		t.Fatalf("expected merchant_id 'auth-merchant' from auth context, got %s", store.MerchantID)
	}
}

func TestCreateStore_MissingMerchantContext(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	req := dto.CreateStoreRequest{
		Name: "Store without context",
	}

	_, err := svc.CreateStore(context.Background(), "", req)
	if err == nil {
		t.Fatal("expected unauthorized error for missing merchant context, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeUnauthorized {
		t.Errorf("expected UNAUTHORIZED code, got %s", appErr.Code)
	}
}

func TestCreateStore_NonMerchantForbidden(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	customerCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "customer-1",
		Role:   auth.RoleCustomer,
	})

	req := dto.CreateStoreRequest{
		Name: "Customer Trying To Open Store",
	}

	_, err := svc.CreateStore(customerCtx, "", req)
	if err == nil {
		t.Fatal("expected forbidden error for customer creating store, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
}

func TestCreateStore_ValidationFailure(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	authCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-1",
	})

	// Empty name
	req := dto.CreateStoreRequest{
		Name: " ",
	}
	_, err := svc.CreateStore(authCtx, "", req)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	if appErrors.AsAppError(err).Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST code, got %s", appErrors.AsAppError(err).Code)
	}

	// Name too long
	reqLong := dto.CreateStoreRequest{
		Name: strings.Repeat("A", 300),
	}
	_, err = svc.CreateStore(authCtx, "", reqLong)
	if err == nil {
		t.Fatal("expected error for too long name, got nil")
	}
}

func TestGetStore_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	store := &model.Store{
		ID:             "store-100",
		MerchantID:     "merchant-100",
		Name:           "Electronics Hub",
		Slug:           "electronics-hub",
		Address:        "123 Tech Lane",
		ApprovalStatus: "APPROVED",
		IsVacationMode: false,
		AvgStoreRating: 4.8,
	}
	_ = repo.Create(context.Background(), store)

	// Get by ID
	res, err := svc.GetStore(context.Background(), "", "store-100")
	if err != nil {
		t.Fatalf("expected get success, got %v", err)
	}
	if res.ID != "store-100" || res.Name != "Electronics Hub" {
		t.Errorf("unexpected store retrieved: %+v", res)
	}

	// Get by MerchantID from auth context
	authCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-100",
	})
	resByMerchant, err := svc.GetStore(authCtx, "merchant-100", "")
	if err != nil {
		t.Fatalf("expected get by merchant success, got %v", err)
	}
	if resByMerchant.ID != "store-100" {
		t.Errorf("unexpected store ID: %s", resByMerchant.ID)
	}
}

func TestGetStore_NonExistent(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	_, err := svc.GetStore(context.Background(), "", "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent store, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeNotFound {
		t.Errorf("expected NOT_FOUND code, got %s", appErr.Code)
	}
}

func TestUpdateStore_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	store := &model.Store{
		ID:             "store-200",
		MerchantID:     "merchant-200",
		Name:           "Fashion World",
		Slug:           "fashion-world",
		Description:    "Old description",
		Address:        "Old address",
		ApprovalStatus: "APPROVED",
		IsVacationMode: false,
	}
	_ = repo.Create(context.Background(), store)

	authCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-200",
		Role:   auth.RoleMerchant,
	})

	newName := "Fashion Universe"
	newEmail := "contact@fashionuniverse.com"
	newPhone := "+1 9876543210"
	newDesc := "Updated trendy fashion storefront"
	newLogo := "https://example.com/new-logo.svg"
	newAddress := "456 Fashion Ave, Suite 500, New York, NY"
	vacation := true

	updateReq := dto.UpdateStoreRequest{
		Name:           &newName,
		BusinessEmail:  &newEmail,
		BusinessPhone:  &newPhone,
		Description:    &newDesc,
		LogoURL:        &newLogo,
		Address:        &newAddress,
		IsVacationMode: &vacation,
	}

	updated, err := svc.UpdateStore(authCtx, "", "store-200", updateReq)
	if err != nil {
		t.Fatalf("expected update success, got %v", err)
	}

	if updated.Name != "Fashion Universe" {
		t.Errorf("expected updated name Fashion Universe, got %s", updated.Name)
	}
	if updated.BusinessEmail != "contact@fashionuniverse.com" {
		t.Errorf("expected updated business email, got %s", updated.BusinessEmail)
	}
	if updated.BusinessPhone != "+1 9876543210" {
		t.Errorf("expected updated business phone, got %s", updated.BusinessPhone)
	}
	if updated.Description != "Updated trendy fashion storefront" {
		t.Errorf("expected updated description, got %s", updated.Description)
	}
	if updated.LogoURL != "https://example.com/new-logo.svg" {
		t.Errorf("expected updated logo, got %s", updated.LogoURL)
	}
	if updated.Address != "456 Fashion Ave, Suite 500, New York, NY" {
		t.Errorf("expected updated address, got %s", updated.Address)
	}
	if !updated.IsVacationMode {
		t.Errorf("expected is_vacation_mode true, got %v", updated.IsVacationMode)
	}
}

func TestUpdateStore_OwnershipEnforcement(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	// Store belongs to "merchant-owner"
	store := &model.Store{
		ID:             "store-300",
		MerchantID:     "merchant-owner",
		Name:           "Owner's Store",
		Slug:           "owners-store",
		ApprovalStatus: "APPROVED",
	}
	_ = repo.Create(context.Background(), store)

	// Attacker tries to modify "merchant-owner"'s store
	attackerCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-attacker",
		Role:   auth.RoleMerchant,
	})

	hackedName := "Hacked Store"
	updateReq := dto.UpdateStoreRequest{
		Name: &hackedName,
	}

	_, err := svc.UpdateStore(attackerCtx, "", "store-300", updateReq)
	if err == nil {
		t.Fatal("expected forbidden error when merchant modifies another merchant's store, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
}

func TestUpdateStore_NonExistentStore(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	authCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-123",
	})

	newName := "New Name"
	updateReq := dto.UpdateStoreRequest{
		Name: &newName,
	}

	_, err := svc.UpdateStore(authCtx, "", "nonexistent-store", updateReq)
	if err == nil {
		t.Fatal("expected error for nonexistent store, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeNotFound {
		t.Errorf("expected NOT_FOUND code, got %s", appErr.Code)
	}
}

func TestListStores(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	s1 := &model.Store{
		ID:         "s1",
		MerchantID: "m1",
		Name:       "Store 1",
		Slug:       "store-1",
	}
	s2 := &model.Store{
		ID:         "s2",
		MerchantID: "m2",
		Name:       "Store 2",
		Slug:       "store-2",
	}
	_ = repo.Create(context.Background(), s1)
	_ = repo.Create(context.Background(), s2)

	listAll, totalAll, err := svc.ListStores(context.Background(), "", 10, 0)
	if err != nil {
		t.Fatalf("expected list success, got %v", err)
	}
	if totalAll != 2 || len(listAll) != 2 {
		t.Errorf("expected 2 stores, got total=%d len=%d", totalAll, len(listAll))
	}

	listM1, totalM1, err := svc.ListStores(context.Background(), "m1", 10, 0)
	if err != nil {
		t.Fatalf("expected list m1 success, got %v", err)
	}
	if totalM1 != 1 || len(listM1) != 1 || listM1[0].MerchantID != "m1" {
		t.Errorf("expected 1 store for m1, got total=%d", totalM1)
	}
}

func TestSubmitStore_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	authCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Draft Store",
		Slug:           "draft-store",
		ApprovalStatus: model.StoreStatusDraft,
	}
	_ = repo.Create(context.Background(), store)

	submitted, err := svc.SubmitStore(authCtx, "", dto.SubmitStoreRequest{
		StoreID: "store-1",
	})
	if err != nil {
		t.Fatalf("expected submit success, got %v", err)
	}

	if submitted.ApprovalStatus != model.StoreStatusPendingApproval {
		t.Errorf("expected status %s, got %s", model.StoreStatusPendingApproval, submitted.ApprovalStatus)
	}
}

func TestSubmitStore_InvalidState(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	authCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Already Approved Store",
		Slug:           "already-approved",
		ApprovalStatus: model.StoreStatusApproved,
	}
	_ = repo.Create(context.Background(), store)

	_, err := svc.SubmitStore(authCtx, "", dto.SubmitStoreRequest{
		StoreID: "store-1",
	})
	if err == nil {
		t.Fatal("expected error submitting already approved store, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeUnprocessableEntity {
		t.Errorf("expected UNPROCESSABLE_ENTITY code, got %s", appErr.Code)
	}
}

func TestSubmitStore_OwnershipEnforcement(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	authCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "attacker-merchant",
		Role:   auth.RoleMerchant,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "victim-merchant",
		Name:           "Victim Store",
		Slug:           "victim-store",
		ApprovalStatus: model.StoreStatusDraft,
	}
	_ = repo.Create(context.Background(), store)

	_, err := svc.SubmitStore(authCtx, "", dto.SubmitStoreRequest{
		StoreID: "store-1",
	})
	if err == nil {
		t.Fatal("expected error submitting another merchant's store, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
}

func TestSubmitStore_MissingAuth(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Store",
		Slug:           "store",
		ApprovalStatus: model.StoreStatusDraft,
	}
	_ = repo.Create(context.Background(), store)

	_, err := svc.SubmitStore(context.Background(), "", dto.SubmitStoreRequest{
		StoreID: "store-1",
	})
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeUnauthorized {
		t.Errorf("expected UNAUTHORIZED code, got %s", appErr.Code)
	}
}

func TestApproveStore_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   auth.RoleAdmin,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Pending Store",
		Slug:           "pending-store",
		ApprovalStatus: model.StoreStatusPendingApproval,
	}
	_ = repo.Create(context.Background(), store)

	approved, err := svc.ApproveStore(adminCtx, "", dto.ApproveStoreRequest{
		StoreID: "store-1",
	})
	if err != nil {
		t.Fatalf("expected approve success, got %v", err)
	}

	if approved.ApprovalStatus != model.StoreStatusApproved {
		t.Errorf("expected status %s, got %s", model.StoreStatusApproved, approved.ApprovalStatus)
	}
}

func TestApproveStore_NonAdminForbidden(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-2",
		Role:   auth.RoleMerchant,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Pending Store",
		Slug:           "pending-store",
		ApprovalStatus: model.StoreStatusPendingApproval,
	}
	_ = repo.Create(context.Background(), store)

	_, err := svc.ApproveStore(merchantCtx, "", dto.ApproveStoreRequest{
		StoreID: "store-1",
	})
	if err == nil {
		t.Fatal("expected forbidden error for non-admin approving store, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
}

func TestApproveStore_MerchantCannotApproveOwnStore(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleAdmin,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Pending Store",
		Slug:           "pending-store",
		ApprovalStatus: model.StoreStatusPendingApproval,
	}
	_ = repo.Create(context.Background(), store)

	_, err := svc.ApproveStore(adminCtx, "", dto.ApproveStoreRequest{
		StoreID: "store-1",
	})
	if err == nil {
		t.Fatal("expected error when merchant tries to approve own store, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
}

func TestApproveStore_InvalidStateTransition(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   auth.RoleAdmin,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Draft Store",
		Slug:           "draft-store",
		ApprovalStatus: model.StoreStatusDraft,
	}
	_ = repo.Create(context.Background(), store)

	_, err := svc.ApproveStore(adminCtx, "", dto.ApproveStoreRequest{
		StoreID: "store-1",
	})
	if err == nil {
		t.Fatal("expected error approving a draft store without submission, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeUnprocessableEntity {
		t.Errorf("expected UNPROCESSABLE_ENTITY code, got %s", appErr.Code)
	}
}

func TestRejectStore_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   auth.RoleAdmin,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Pending Store",
		Slug:           "pending-store",
		ApprovalStatus: model.StoreStatusPendingApproval,
	}
	_ = repo.Create(context.Background(), store)

	rejected, err := svc.RejectStore(adminCtx, "", dto.RejectStoreRequest{
		StoreID: "store-1",
		Reason:  "KYC documents were blurry and unreadable.",
	})
	if err != nil {
		t.Fatalf("expected reject success, got %v", err)
	}

	if rejected.ApprovalStatus != model.StoreStatusRejected {
		t.Errorf("expected status %s, got %s", model.StoreStatusRejected, rejected.ApprovalStatus)
	}
	if rejected.RejectionReason == nil || *rejected.RejectionReason != "KYC documents were blurry and unreadable." {
		t.Errorf("expected rejection reason persisted, got %v", rejected.RejectionReason)
	}
}

func TestRejectStore_MissingReason(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   auth.RoleAdmin,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Pending Store",
		Slug:           "pending-store",
		ApprovalStatus: model.StoreStatusPendingApproval,
	}
	_ = repo.Create(context.Background(), store)

	_, err := svc.RejectStore(adminCtx, "", dto.RejectStoreRequest{
		StoreID: "store-1",
		Reason:  "   ",
	})
	if err == nil {
		t.Fatal("expected error rejecting without reason, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST code, got %s", appErr.Code)
	}
}

func TestRejectStore_NonAdminForbidden(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-2",
		Role:   auth.RoleMerchant,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Pending Store",
		Slug:           "pending-store",
		ApprovalStatus: model.StoreStatusPendingApproval,
	}
	_ = repo.Create(context.Background(), store)

	_, err := svc.RejectStore(merchantCtx, "", dto.RejectStoreRequest{
		StoreID: "store-1",
		Reason:  "Rejection reason",
	})
	if err == nil {
		t.Fatal("expected forbidden error for non-admin rejecting store, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
}

func TestRejectStore_MerchantCannotRejectOwnStore(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleAdmin,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Pending Store",
		Slug:           "pending-store",
		ApprovalStatus: model.StoreStatusPendingApproval,
	}
	_ = repo.Create(context.Background(), store)

	_, err := svc.RejectStore(adminCtx, "", dto.RejectStoreRequest{
		StoreID: "store-1",
		Reason:  "Some reason",
	})
	if err == nil {
		t.Fatal("expected error when merchant tries to reject own store, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
}

func TestRejectStore_InvalidStateTransition(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   auth.RoleAdmin,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Draft Store",
		Slug:           "draft-store",
		ApprovalStatus: model.StoreStatusDraft,
	}
	_ = repo.Create(context.Background(), store)

	_, err := svc.RejectStore(adminCtx, "", dto.RejectStoreRequest{
		StoreID: "store-1",
		Reason:  "Invalid documents",
	})
	if err == nil {
		t.Fatal("expected error rejecting draft store without submission, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeUnprocessableEntity {
		t.Errorf("expected UNPROCESSABLE_ENTITY code, got %s", appErr.Code)
	}
}
