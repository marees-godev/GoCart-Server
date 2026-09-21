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

func TestCreateStore_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)

	authCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-123",
		Role:   "MERCHANT",
	})

	bankDetails := `{"account_number":"123456789","bank_name":"Test Bank"}`
	req := dto.CreateStoreRequest{
		Name:               "Happy Shop",
		Description:        "At Happy Shop, we believe shopping should be simple, smart, and satisfying.",
		LogoURL:            "https://example.com/logo.png",
		BannerURL:          "https://example.com/banner.png",
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
	if store.ApprovalStatus != "PENDING" {
		t.Errorf("expected status PENDING, got %s", store.ApprovalStatus)
	}
	if store.PublishStatus != false {
		t.Errorf("expected publish_status false, got %v", store.PublishStatus)
	}
	if store.KYCStatus != "PENDING" {
		t.Errorf("expected kyc_status PENDING, got %s", store.KYCStatus)
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
		Role:   "MERCHANT",
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
		PublishStatus:  true,
		KYCStatus:      "VERIFIED",
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
		PublishStatus:  false,
		KYCStatus:      "VERIFIED",
	}
	_ = repo.Create(context.Background(), store)

	authCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-200",
		Role:   "MERCHANT",
	})

	newName := "Fashion Universe"
	newDesc := "Updated trendy fashion storefront"
	newLogo := "https://example.com/new-logo.svg"
	newBanner := "https://example.com/new-banner.jpg"
	newAddress := "456 Fashion Ave, Suite 500, New York, NY"
	publish := true

	updateReq := dto.UpdateStoreRequest{
		Name:          &newName,
		Description:   &newDesc,
		LogoURL:       &newLogo,
		BannerURL:     &newBanner,
		Address:       &newAddress,
		PublishStatus: &publish,
	}

	updated, err := svc.UpdateStore(authCtx, "", "store-200", updateReq)
	if err != nil {
		t.Fatalf("expected update success, got %v", err)
	}

	if updated.Name != "Fashion Universe" {
		t.Errorf("expected updated name Fashion Universe, got %s", updated.Name)
	}
	if updated.Description != "Updated trendy fashion storefront" {
		t.Errorf("expected updated description, got %s", updated.Description)
	}
	if updated.LogoURL != "https://example.com/new-logo.svg" {
		t.Errorf("expected updated logo, got %s", updated.LogoURL)
	}
	if updated.BannerURL != "https://example.com/new-banner.jpg" {
		t.Errorf("expected updated banner, got %s", updated.BannerURL)
	}
	if updated.Address != "456 Fashion Ave, Suite 500, New York, NY" {
		t.Errorf("expected updated address, got %s", updated.Address)
	}
	if !updated.PublishStatus {
		t.Errorf("expected publish_status true, got %v", updated.PublishStatus)
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
		Role:   "MERCHANT",
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
