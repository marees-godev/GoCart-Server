package handler_test

import (
	"context"
	"testing"

	storepb "github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockStoreRepo struct {
	stores map[string]*model.Store
	slugs  map[string]string
}

func newMockRepo() *mockStoreRepo {
	return &mockStoreRepo{
		stores: make(map[string]*model.Store),
		slugs:  make(map[string]string),
	}
}

func (m *mockStoreRepo) Create(ctx context.Context, s *model.Store) error {
	if s.ID == "" {
		s.ID = "store-" + s.Slug
	}
	m.stores[s.ID] = s
	m.slugs[s.Slug] = s.ID
	return nil
}

func (m *mockStoreRepo) GetByID(ctx context.Context, id string) (*model.Store, error) {
	s, ok := m.stores[id]
	if !ok {
		return nil, appErrors.NotFound("store not found")
	}
	cp := *s
	return &cp, nil
}

func (m *mockStoreRepo) GetByMerchantID(ctx context.Context, merchantID string) (*model.Store, error) {
	for _, s := range m.stores {
		if s.MerchantID == merchantID {
			cp := *s
			return &cp, nil
		}
	}
	return nil, appErrors.NotFound("store not found")
}

func (m *mockStoreRepo) GetBySlug(ctx context.Context, slug string) (*model.Store, error) {
	id, ok := m.slugs[slug]
	if !ok {
		return nil, appErrors.NotFound("store not found")
	}
	cp := *m.stores[id]
	return &cp, nil
}

func (m *mockStoreRepo) List(ctx context.Context, merchantID string, limit, offset int) ([]*model.Store, int, error) {
	res := make([]*model.Store, 0)
	for _, s := range m.stores {
		if merchantID == "" || s.MerchantID == merchantID {
			cp := *s
			res = append(res, &cp)
		}
	}
	return res, len(res), nil
}

func (m *mockStoreRepo) Update(ctx context.Context, s *model.Store) error {
	existing, ok := m.stores[s.ID]
	if !ok {
		return appErrors.NotFound("store not found")
	}
	if existing.Slug != s.Slug {
		delete(m.slugs, existing.Slug)
		m.slugs[s.Slug] = s.ID
	}
	m.stores[s.ID] = s
	return nil
}

func (m *mockStoreRepo) IsSlugAvailable(ctx context.Context, slug string, excludeID string) (bool, error) {
	id, ok := m.slugs[slug]
	if !ok {
		return true, nil
	}
	if excludeID != "" && id == excludeID {
		return true, nil
	}
	return false, nil
}

func (m *mockStoreRepo) UpdateStatus(ctx context.Context, id string, expectedStatus, newStatus string, rejectionReason *string) (*model.Store, error) {
	existing, ok := m.stores[id]
	if !ok {
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

func TestGRPCHandler_CreateStore(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)
	h := handler.NewStoreGRPCHandler(svc)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-10",
		Role:   auth.RoleMerchant,
	})

	req := &storepb.CreateStoreRequest{
		Name:        "Gadget Store",
		Description: "Latest smartphones and laptops",
		Address:     "100 Tech Blvd",
	}

	resp, err := h.CreateStore(ctx, req)
	if err != nil {
		t.Fatalf("expected CreateStore success, got %v", err)
	}
	if resp.Store == nil || resp.Store.Name != "Gadget Store" || resp.Store.MerchantId != "merchant-10" {
		t.Errorf("unexpected store in response: %+v", resp.Store)
	}
}

func TestGRPCHandler_GetStore_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)
	h := handler.NewStoreGRPCHandler(svc)

	req := &storepb.GetStoreRequest{
		Id: "non-existent",
	}

	_, err := h.GetStore(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Errorf("expected NOT_FOUND status, got %v", err)
	}
}

func TestGRPCHandler_UpdateStore_OwnershipDenied(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)
	h := handler.NewStoreGRPCHandler(svc)

	_ = repo.Create(context.Background(), &model.Store{
		ID:         "store-55",
		MerchantID: "merchant-legit",
		Name:       "Legit Store",
		Slug:       "legit-store",
	})

	// Attacker calling gRPC
	attackerCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-hacker",
	})

	newName := "Hijacked Store"
	req := &storepb.UpdateStoreRequest{
		Id:   "store-55",
		Name: &newName,
	}

	_, err := h.UpdateStore(attackerCtx, req)
	if err == nil {
		t.Fatal("expected PermissionDenied error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.PermissionDenied {
		t.Errorf("expected PERMISSION_DENIED status, got %v", err)
	}
}

func TestGRPCHandler_ListStores(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)
	h := handler.NewStoreGRPCHandler(svc)

	_ = repo.Create(context.Background(), &model.Store{
		ID:         "s1",
		MerchantID: "m1",
		Name:       "Store 1",
		Slug:       "store-1",
	})

	resp, err := h.ListStores(context.Background(), &storepb.ListStoresRequest{
		MerchantId: "m1",
	})
	if err != nil {
		t.Fatalf("expected ListStores success, got %v", err)
	}
	if resp.Total != 1 || len(resp.Stores) != 1 {
		t.Errorf("expected 1 store, got total=%d", resp.Total)
	}
}

func TestGRPCHandler_SubmitStore(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)
	h := handler.NewStoreGRPCHandler(svc)

	_ = repo.Create(context.Background(), &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Draft Store",
		Slug:           "draft-store",
		ApprovalStatus: model.StoreStatusDraft,
	})

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})

	resp, err := h.SubmitStore(ctx, &storepb.SubmitStoreRequest{
		StoreId: "store-1",
	})
	if err != nil {
		t.Fatalf("expected SubmitStore success, got %v", err)
	}
	if resp.Store == nil || resp.Store.ApprovalStatus != model.StoreStatusPendingApproval {
		t.Errorf("expected status %s, got %v", model.StoreStatusPendingApproval, resp.Store)
	}
}

func TestGRPCHandler_ApproveStore(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)
	h := handler.NewStoreGRPCHandler(svc)

	_ = repo.Create(context.Background(), &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Pending Store",
		Slug:           "pending-store",
		ApprovalStatus: model.StoreStatusPendingApproval,
	})

	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   auth.RoleAdmin,
	})

	resp, err := h.ApproveStore(adminCtx, &storepb.ApproveStoreRequest{
		StoreId: "store-1",
	})
	if err != nil {
		t.Fatalf("expected ApproveStore success, got %v", err)
	}
	if resp.Store == nil || resp.Store.ApprovalStatus != model.StoreStatusApproved {
		t.Errorf("expected status %s, got %v", model.StoreStatusApproved, resp.Store)
	}
}

func TestGRPCHandler_RejectStore(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)
	h := handler.NewStoreGRPCHandler(svc)

	_ = repo.Create(context.Background(), &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Pending Store",
		Slug:           "pending-store",
		ApprovalStatus: model.StoreStatusPendingApproval,
	})

	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   auth.RoleAdmin,
	})

	resp, err := h.RejectStore(adminCtx, &storepb.RejectStoreRequest{
		StoreId:         "store-1",
		RejectionReason: "Incomplete bank details",
	})
	if err != nil {
		t.Fatalf("expected RejectStore success, got %v", err)
	}
	if resp.Store == nil || resp.Store.ApprovalStatus != model.StoreStatusRejected {
		t.Errorf("expected status %s, got %v", model.StoreStatusRejected, resp.Store)
	}
	if resp.Store.RejectionReason != "Incomplete bank details" {
		t.Errorf("expected rejection reason to match, got %s", resp.Store.RejectionReason)
	}
}

func TestGRPCHandler_RejectStore_MissingReason(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)
	h := handler.NewStoreGRPCHandler(svc)

	_ = repo.Create(context.Background(), &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Pending Store",
		Slug:           "pending-store",
		ApprovalStatus: model.StoreStatusPendingApproval,
	})

	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   auth.RoleAdmin,
	})

	_, err := h.RejectStore(adminCtx, &storepb.RejectStoreRequest{
		StoreId:         "store-1",
		RejectionReason: "",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected INVALID_ARGUMENT status, got %v", err)
	}
}

func TestGRPCHandler_ApproveStore_SelfApprovalDenied(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewStoreService(repo)
	h := handler.NewStoreGRPCHandler(svc)

	_ = repo.Create(context.Background(), &model.Store{
		ID:             "store-1",
		MerchantID:     "merchant-1",
		Name:           "Pending Store",
		Slug:           "pending-store",
		ApprovalStatus: model.StoreStatusPendingApproval,
	})

	merchantAsAdminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleAdmin,
	})

	_, err := h.ApproveStore(merchantAsAdminCtx, &storepb.ApproveStoreRequest{
		StoreId: "store-1",
	})
	if err == nil {
		t.Fatal("expected error when merchant approves own store, got nil")
	}

	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.PermissionDenied {
		t.Errorf("expected PERMISSION_DENIED status, got %v", err)
	}
}

