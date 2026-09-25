package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/client/gstin"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/service"
)

type mockStoreRepository struct {
	stores  map[string]*model.Store
	slugs   map[string]string
	appeals map[string]*model.StoreAppeal
}

func newMockRepo() *mockStoreRepository {
	return &mockStoreRepository{
		stores:  make(map[string]*model.Store),
		slugs:   make(map[string]string),
		appeals: make(map[string]*model.StoreAppeal),
	}
}

func (m *mockStoreRepository) CreateAppeal(ctx context.Context, appeal *model.StoreAppeal) error {
	if appeal.ID == "" {
		appeal.ID = "appeal-" + appeal.StoreID
	}
	appeal.CreatedAt = time.Now()
	appeal.UpdatedAt = time.Now()
	m.appeals[appeal.ID] = appeal
	return nil
}

func (m *mockStoreRepository) GetPendingAppealByStoreID(ctx context.Context, storeID string) (*model.StoreAppeal, error) {
	for _, a := range m.appeals {
		if a.StoreID == storeID && a.Status == model.AppealStatusPending {
			cp := *a
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockStoreRepository) GetAppealByID(ctx context.Context, id string) (*model.StoreAppeal, error) {
	a, exists := m.appeals[id]
	if !exists {
		return nil, appErrors.NotFound("appeal not found")
	}
	cp := *a
	return &cp, nil
}

func (m *mockStoreRepository) ListAppealsByStoreID(ctx context.Context, storeID string) ([]*model.StoreAppeal, error) {
	res := make([]*model.StoreAppeal, 0)
	for _, a := range m.appeals {
		if a.StoreID == storeID {
			cp := *a
			res = append(res, &cp)
		}
	}
	return res, nil
}

func (m *mockStoreRepository) UpdateAppealStatus(ctx context.Context, id string, status string, adminComment *string) (*model.StoreAppeal, error) {
	a, exists := m.appeals[id]
	if !exists {
		return nil, appErrors.NotFound("appeal not found")
	}
	a.Status = status
	a.AdminComment = adminComment
	now := time.Now()
	a.ReviewedAt = &now
	a.UpdatedAt = now
	m.appeals[id] = a
	cp := *a
	return &cp, nil
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
	m.stores[id] = existing
	cp := *existing
	return &cp, nil
}

func (m *mockStoreRepository) UpdateApprovalStatus(ctx context.Context, id string, newStatus string, rejectionReason *string) (*model.Store, error) {
	existing, exists := m.stores[id]
	if !exists {
		return nil, appErrors.NotFound("store not found")
	}
	existing.ApprovalStatus = newStatus
	existing.RejectionReason = rejectionReason
	if newStatus == model.StoreStatusSuspended || newStatus == model.StoreStatusClosed {
		existing.IsPublished = false
	}
	existing.UpdatedAt = time.Now()
	m.stores[id] = existing
	cp := *existing
	return &cp, nil
}

func (m *mockStoreRepository) SubmitKYC(ctx context.Context, storeID string, kycStatus string, bankAccount *model.StoreBankAccount) (*model.Store, error) {
	existing, exists := m.stores[storeID]
	if !exists {
		return nil, appErrors.NotFound("store not found")
	}
	existing.KYCStatus = kycStatus
	existing.BankAccount = bankAccount
	if bankAccount != nil && bankAccount.BusinessRegistration != nil {
		existing.BusinessRegistration = bankAccount.BusinessRegistration
	}
	existing.UpdatedAt = time.Now()
	m.stores[storeID] = existing
	cp := *existing
	return &cp, nil
}

type mockGSTINClient struct {
	verifyFunc func(ctx context.Context, gstinStr string) (*gstin.GSTINResponse, error)
}

func (m *mockGSTINClient) VerifyGSTIN(ctx context.Context, gstinStr string) (*gstin.GSTINResponse, error) {
	if m.verifyFunc != nil {
		return m.verifyFunc(ctx, gstinStr)
	}
	return &gstin.GSTINResponse{
		Success: true,
		GSTIN:   gstinStr,
		Data: &gstin.GSTINData{
			GSTIN:       gstinStr,
			Status:      "Active",
			BlockStatus: "Unblocked",
		},
	}, nil
}

func (m *mockStoreRepository) SetPublishStatus(ctx context.Context, storeID string, isPublished bool) (*model.Store, error) {
	existing, exists := m.stores[storeID]
	if !exists {
		return nil, appErrors.NotFound("store not found")
	}
	existing.IsPublished = isPublished
	existing.UpdatedAt = time.Now()
	m.stores[storeID] = existing
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

func TestSubmitKYC_Success(t *testing.T) {
	repo := newMockRepo()
	mockClient := &mockGSTINClient{}
	svc := service.NewStoreService(repo, mockClient)

	authCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Test Store",
		Slug:           "test-store",
		ApprovalStatus: model.StoreStatusApproved,
	}
	_ = repo.Create(context.Background(), store)

	gstinVal := "33AAACC1206D1ZN"
	taxVal := "TAX-7890"
	req := dto.SubmitKYCRequest{
		StoreID:              "store-1",
		BusinessRegistration: "REG-123456",
		TaxID:                &taxVal,
		BankName:             "Chase Bank",
		AccountNumber:        "1234567890",
		AccountHolderName:    "Merchant Owner",
		GSTIN:                &gstinVal,
	}

	updated, err := svc.SubmitKYC(authCtx, "", req)
	if err != nil {
		t.Fatalf("expected SubmitKYC success, got %v", err)
	}

	if updated.KYCStatus != model.KYCStatusPending {
		t.Errorf("expected KYC status %s, got %s", model.KYCStatusPending, updated.KYCStatus)
	}
	if updated.BusinessRegistration == nil || *updated.BusinessRegistration != "REG-123456" {
		t.Errorf("expected business registration REG-123456, got %v", updated.BusinessRegistration)
	}
}

func TestSubmitKYC_OwnershipEnforcement(t *testing.T) {
	repo := newMockRepo()
	mockClient := &mockGSTINClient{}
	svc := service.NewStoreService(repo, mockClient)

	attackerCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "attacker-merchant",
		Role:   auth.RoleMerchant,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "victim-merchant",
		Name:           "Victim Store",
		Slug:           "victim-store",
		ApprovalStatus: model.StoreStatusApproved,
	}
	_ = repo.Create(context.Background(), store)

	gstinVal := "33AAACC1206D1ZN"
	taxVal := "TAX-999"
	req := dto.SubmitKYCRequest{
		StoreID:              "store-1",
		BusinessRegistration: "REG-999",
		TaxID:                &taxVal,
		BankName:             "Fake Bank",
		AccountNumber:        "999999",
		AccountHolderName:    "Attacker",
		GSTIN:                &gstinVal,
	}

	_, err := svc.SubmitKYC(attackerCtx, "", req)
	if err == nil {
		t.Fatal("expected error submitting KYC for another merchant's store, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
}

func TestPublishStore_Rules(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	authCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	// 1. Draft/Unapproved store cannot be published
	draftStore := &model.Store{
		ID:             "store-draft",
		MerchantID:     "merchant-1",
		ApprovalStatus: model.StoreStatusDraft,
		IsPublished:    false,
	}
	_ = repo.Create(context.Background(), draftStore)

	_, err := svc.PublishStore(authCtx, "", dto.PublishStoreRequest{StoreID: "store-draft"})
	if err == nil {
		t.Fatal("expected error publishing draft store, got nil")
	}
	if appErrors.AsAppError(err).Code != appErrors.CodeUnprocessableEntity {
		t.Errorf("expected UNPROCESSABLE_ENTITY, got %s", appErrors.AsAppError(err).Code)
	}

	// 2. Rejected store cannot be published
	rejectedStore := &model.Store{
		ID:             "store-rejected",
		MerchantID:     "merchant-1",
		ApprovalStatus: model.StoreStatusRejected,
		IsPublished:    false,
	}
	_ = repo.Create(context.Background(), rejectedStore)

	_, err = svc.PublishStore(authCtx, "", dto.PublishStoreRequest{StoreID: "store-rejected"})
	if err == nil {
		t.Fatal("expected error publishing rejected store, got nil")
	}

	// 3. Approved store can be published
	approvedStore := &model.Store{
		ID:             "store-approved",
		MerchantID:     "merchant-1",
		ApprovalStatus: model.StoreStatusApproved,
		IsPublished:    false,
	}
	_ = repo.Create(context.Background(), approvedStore)

	published, err := svc.PublishStore(authCtx, "", dto.PublishStoreRequest{StoreID: "store-approved"})
	if err != nil {
		t.Fatalf("expected publish success for approved store, got %v", err)
	}
	if !published.IsPublished {
		t.Error("expected IsPublished to be true")
	}

	// 4. Approved store can be unpublished
	unpublished, err := svc.UnpublishStore(authCtx, "", dto.UnpublishStoreRequest{StoreID: "store-approved"})
	if err != nil {
		t.Fatalf("expected unpublish success, got %v", err)
	}
	if unpublished.IsPublished {
		t.Error("expected IsPublished to be false")
	}
}

func TestSuspendStore_Rules(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   auth.RoleAdmin,
	})

	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		ApprovalStatus: model.StoreStatusApproved,
		IsPublished:    true,
	}
	_ = repo.Create(context.Background(), store)

	// Merchant cannot suspend store (Admin-only protected)
	_, err := svc.SuspendStore(merchantCtx, "", dto.SuspendStoreRequest{
		StoreID: "store-1",
		Reason:  "Illegal goods",
	})
	if err == nil {
		t.Fatal("expected forbidden for merchant suspending store, got nil")
	}

	// Admin suspends store
	suspended, err := svc.SuspendStore(adminCtx, "", dto.SuspendStoreRequest{
		StoreID: "store-1",
		Reason:  "Policy violation",
	})
	if err != nil {
		t.Fatalf("expected suspend success for admin, got %v", err)
	}

	if suspended.ApprovalStatus != model.StoreStatusSuspended {
		t.Errorf("expected status %s, got %s", model.StoreStatusSuspended, suspended.ApprovalStatus)
	}
	if suspended.IsPublished {
		t.Error("suspended store must be unpublished (is_published = false)")
	}

	// Suspended store cannot be published by merchant
	_, err = svc.PublishStore(merchantCtx, "", dto.PublishStoreRequest{StoreID: "store-1"})
	if err == nil {
		t.Fatal("expected error publishing suspended store, got nil")
	}
}

func TestUnsuspendStore_Rules(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   auth.RoleAdmin,
	})

	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	store := &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		ApprovalStatus: model.StoreStatusSuspended,
		IsPublished:    false,
	}
	_ = repo.Create(context.Background(), store)

	// Submit an appeal
	_, appeal, err := svc.AppealStore(merchantCtx, "", dto.AppealStoreRequest{
		StoreID: "store-1",
		Reason:  "Please unblock me",
	})
	if err != nil {
		t.Fatalf("expected appeal creation success, got %v", err)
	}
	if appeal.Status != model.AppealStatusPending {
		t.Fatalf("expected PENDING status for new appeal, got %s", appeal.Status)
	}

	// Merchant cannot unsuspend (Admin only)
	_, err = svc.UnsuspendStore(merchantCtx, "", dto.UnsuspendStoreRequest{
		StoreID: "store-1",
	})
	if err == nil {
		t.Fatal("expected forbidden error for merchant unsuspending store, got nil")
	}

	// Admin unsuspends store with a reason
	unsuspended, err := svc.UnsuspendStore(adminCtx, "", dto.UnsuspendStoreRequest{
		StoreID: "store-1",
		Reason:  "it's ok bro",
	})
	if err != nil {
		t.Fatalf("expected unsuspend success for admin, got %v", err)
	}

	if unsuspended.ApprovalStatus != model.StoreStatusApproved {
		t.Errorf("expected status %s, got %s", model.StoreStatusApproved, unsuspended.ApprovalStatus)
	}

	// Verify pending appeal is marked APPROVED with admin_comment
	updatedAppeal, err := repo.GetAppealByID(context.Background(), appeal.ID)
	if err != nil {
		t.Fatalf("expected appeal to exist, got %v", err)
	}
	if updatedAppeal.Status != model.AppealStatusApproved {
		t.Errorf("expected appeal status APPROVED, got %s", updatedAppeal.Status)
	}
	if updatedAppeal.AdminComment == nil || *updatedAppeal.AdminComment != "it's ok bro" {
		t.Errorf("expected admin comment 'it's ok bro', got %v", updatedAppeal.AdminComment)
	}

	// Unsuspending a non-suspended store should fail
	_, err = svc.UnsuspendStore(adminCtx, "", dto.UnsuspendStoreRequest{
		StoreID: "store-1",
	})
	if err == nil {
		t.Fatal("expected error unsuspending non-suspended store, got nil")
	}
}

func TestAppealStore_Rules(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	attackerCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "attacker-merchant",
		Role:   auth.RoleMerchant,
	})

	rejReason := "Violated terms of service"
	store := &model.Store{
		ID:              "store-suspended",
		MerchantID:      "merchant-1",
		ApprovalStatus:  model.StoreStatusSuspended,
		RejectionReason: &rejReason,
		IsPublished:     false,
	}
	_ = repo.Create(context.Background(), store)

	// Attacker cannot appeal another merchant's store
	_, _, err := svc.AppealStore(attackerCtx, "", dto.AppealStoreRequest{
		StoreID: "store-suspended",
		Reason:  "Please restore my store",
	})
	if err == nil {
		t.Fatal("expected forbidden error for appealing another merchant's store, got nil")
	}

	// Owner appeals suspended store
	st, appeal, err := svc.AppealStore(merchantCtx, "", dto.AppealStoreRequest{
		StoreID: "store-suspended",
		Reason:  "I have resolved all compliance issues",
	})
	if err != nil {
		t.Fatalf("expected appeal success for owner, got %v", err)
	}

	// Store status remains SUSPENDED and rejection_reason stays intact!
	if st.ApprovalStatus != model.StoreStatusSuspended {
		t.Errorf("expected store approval status to remain SUSPENDED, got %s", st.ApprovalStatus)
	}
	if st.RejectionReason == nil || *st.RejectionReason != "Violated terms of service" {
		t.Errorf("expected original rejection_reason to remain intact, got %v", st.RejectionReason)
	}

	if appeal.Status != model.AppealStatusPending {
		t.Errorf("expected appeal status PENDING, got %s", appeal.Status)
	}
	if appeal.Reason != "I have resolved all compliance issues" {
		t.Errorf("unexpected appeal reason: %s", appeal.Reason)
	}

	// Submitting duplicate pending appeal fails
	_, _, err = svc.AppealStore(merchantCtx, "", dto.AppealStoreRequest{
		StoreID: "store-suspended",
		Reason:  "Duplicate appeal",
	})
	if err == nil {
		t.Fatal("expected conflict error for submitting duplicate pending appeal, got nil")
	}
}
