package tests_test

import (
	"context"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/service"
)

type mockStateRepo struct {
	stores  map[string]*model.Store
	appeals map[string]*model.StoreAppeal
}

func newMockStateRepo() *mockStateRepo {
	return &mockStateRepo{
		stores:  make(map[string]*model.Store),
		appeals: make(map[string]*model.StoreAppeal),
	}
}

func (m *mockStateRepo) Create(ctx context.Context, store *model.Store) error {
	if store.ID == "" {
		store.ID = "store-1"
	}
	m.stores[store.ID] = store
	return nil
}

func (m *mockStateRepo) GetByID(ctx context.Context, id string) (*model.Store, error) {
	s, ok := m.stores[id]
	if !ok {
		return nil, appErrors.NotFound("store not found")
	}
	cp := *s
	return &cp, nil
}

func (m *mockStateRepo) GetByMerchantID(ctx context.Context, merchantID string) (*model.Store, error) {
	for _, s := range m.stores {
		if s.MerchantID == merchantID {
			cp := *s
			return &cp, nil
		}
	}
	return nil, appErrors.NotFound("store not found")
}

func (m *mockStateRepo) GetBySlug(ctx context.Context, slug string) (*model.Store, error) {
	for _, s := range m.stores {
		if s.Slug == slug {
			cp := *s
			return &cp, nil
		}
	}
	return nil, appErrors.NotFound("store not found")
}

func (m *mockStateRepo) List(ctx context.Context, merchantID string, limit, offset int) ([]*model.Store, int, error) {
	res := make([]*model.Store, 0)
	for _, s := range m.stores {
		if merchantID == "" || s.MerchantID == merchantID {
			cp := *s
			res = append(res, &cp)
		}
	}
	return res, len(res), nil
}

func (m *mockStateRepo) Update(ctx context.Context, store *model.Store) error {
	m.stores[store.ID] = store
	return nil
}

func (m *mockStateRepo) UpdateStatus(ctx context.Context, id string, expectedStatus, newStatus string, rejectionReason *string, actorID ...string) (*model.Store, error) {
	s, ok := m.stores[id]
	if !ok {
		return nil, appErrors.NotFound("store not found")
	}
	if s.ApprovalStatus != expectedStatus {
		return nil, appErrors.UnprocessableEntity("invalid state transition")
	}
	s.ApprovalStatus = newStatus
	s.RejectionReason = rejectionReason
	m.stores[id] = s
	cp := *s
	return &cp, nil
}

func (m *mockStateRepo) UpdateApprovalStatus(ctx context.Context, id string, newStatus string, rejectionReason *string, actorID ...string) (*model.Store, error) {
	s, ok := m.stores[id]
	if !ok {
		return nil, appErrors.NotFound("store not found")
	}
	s.ApprovalStatus = newStatus
	s.RejectionReason = rejectionReason
	if newStatus == model.StoreStatusSuspended || newStatus == model.StoreStatusClosed {
		s.IsPublished = false
	}
	m.stores[id] = s
	cp := *s
	return &cp, nil
}

func (m *mockStateRepo) SubmitKYC(ctx context.Context, storeID string, kycStatus string, bankAccount *model.StoreBankAccount) (*model.Store, error) {
	s, ok := m.stores[storeID]
	if !ok {
		return nil, appErrors.NotFound("store not found")
	}
	s.KYCStatus = kycStatus
	s.BankAccount = bankAccount
	m.stores[storeID] = s
	cp := *s
	return &cp, nil
}

func (m *mockStateRepo) SetPublishStatus(ctx context.Context, storeID string, isPublished bool) (*model.Store, error) {
	s, ok := m.stores[storeID]
	if !ok {
		return nil, appErrors.NotFound("store not found")
	}
	s.IsPublished = isPublished
	m.stores[storeID] = s
	cp := *s
	return &cp, nil
}

func (m *mockStateRepo) IsSlugAvailable(ctx context.Context, slug string, excludeID string) (bool, error) {
	for _, s := range m.stores {
		if s.Slug == slug && s.ID != excludeID {
			return false, nil
		}
	}
	return true, nil
}

func (m *mockStateRepo) CreateAppeal(ctx context.Context, appeal *model.StoreAppeal) error {
	appeal.ID = "appeal-1"
	m.appeals[appeal.ID] = appeal
	return nil
}

func (m *mockStateRepo) GetPendingAppealByStoreID(ctx context.Context, storeID string) (*model.StoreAppeal, error) {
	for _, a := range m.appeals {
		if a.StoreID == storeID && a.Status == model.AppealStatusPending {
			cp := *a
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockStateRepo) GetAppealByID(ctx context.Context, id string) (*model.StoreAppeal, error) {
	a, ok := m.appeals[id]
	if !ok {
		return nil, appErrors.NotFound("appeal not found")
	}
	cp := *a
	return &cp, nil
}

func (m *mockStateRepo) ListAppealsByStoreID(ctx context.Context, storeID string) ([]*model.StoreAppeal, error) {
	res := make([]*model.StoreAppeal, 0)
	for _, a := range m.appeals {
		if a.StoreID == storeID {
			cp := *a
			res = append(res, &cp)
		}
	}
	return res, nil
}

func (m *mockStateRepo) UpdateAppealStatus(ctx context.Context, id string, status string, adminComment *string) (*model.StoreAppeal, error) {
	a, ok := m.appeals[id]
	if !ok {
		return nil, appErrors.NotFound("appeal not found")
	}
	a.Status = status
	a.AdminComment = adminComment
	now := time.Now()
	a.ReviewedAt = &now
	m.appeals[id] = a
	cp := *a
	return &cp, nil
}

func TestStateMachine_ValidTransitions(t *testing.T) {
	repo := newMockStateRepo()
	svc := service.NewStoreService(repo)

	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-10",
		Role:   auth.RoleMerchant,
	})

	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-99",
		Role:   auth.RoleAdmin,
	})

	// 1. Create store -> initial status DRAFT
	store, err := svc.CreateStore(merchantCtx, "", dto.CreateStoreRequest{
		Name: "Cycle Store",
	})
	if err != nil {
		t.Fatalf("failed creating store: %v", err)
	}
	if store.ApprovalStatus != model.StoreStatusDraft {
		t.Fatalf("expected status DRAFT, got %s", store.ApprovalStatus)
	}

	// 2. DRAFT -> PENDING_APPROVAL via SubmitStore
	submitted, err := svc.SubmitStore(merchantCtx, "", dto.SubmitStoreRequest{
		StoreID: store.ID,
	})
	if err != nil {
		t.Fatalf("failed submitting store: %v", err)
	}
	if submitted.ApprovalStatus != model.StoreStatusPendingApproval {
		t.Fatalf("expected PENDING_APPROVAL, got %s", submitted.ApprovalStatus)
	}

	// 3. PENDING_APPROVAL -> APPROVED via ApproveStore
	approved, err := svc.ApproveStore(adminCtx, "", dto.ApproveStoreRequest{
		StoreID: store.ID,
	})
	if err != nil {
		t.Fatalf("failed approving store: %v", err)
	}
	if approved.ApprovalStatus != model.StoreStatusApproved {
		t.Fatalf("expected APPROVED, got %s", approved.ApprovalStatus)
	}

	// 4. APPROVED -> PUBLISHED via PublishStore
	published, err := svc.PublishStore(merchantCtx, "", dto.PublishStoreRequest{
		StoreID: store.ID,
	})
	if err != nil {
		t.Fatalf("failed publishing store: %v", err)
	}
	if !published.IsPublished {
		t.Fatalf("expected is_published to be true")
	}

	// 5. APPROVED -> SUSPENDED via SuspendStore (also auto-unpublishes)
	suspended, err := svc.SuspendStore(adminCtx, "", dto.SuspendStoreRequest{
		StoreID: store.ID,
		Reason:  "Policy violation",
	})
	if err != nil {
		t.Fatalf("failed suspending store: %v", err)
	}
	if suspended.ApprovalStatus != model.StoreStatusSuspended {
		t.Fatalf("expected SUSPENDED, got %s", suspended.ApprovalStatus)
	}
	if suspended.IsPublished {
		t.Fatalf("expected is_published to be false when suspended")
	}

	// 6. SUSPENDED -> AppealStore
	_, appeal, err := svc.AppealStore(merchantCtx, "", dto.AppealStoreRequest{
		StoreID: store.ID,
		Reason:  "Corrective actions taken",
	})
	if err != nil {
		t.Fatalf("failed appealing store: %v", err)
	}
	if appeal.Status != model.AppealStatusPending {
		t.Fatalf("expected appeal status PENDING, got %s", appeal.Status)
	}

	// 7. SUSPENDED -> APPROVED via UnsuspendStore
	unsuspended, err := svc.UnsuspendStore(adminCtx, "", dto.UnsuspendStoreRequest{
		StoreID: store.ID,
		Reason:  "Appeal accepted",
	})
	if err != nil {
		t.Fatalf("failed unsuspending store: %v", err)
	}
	if unsuspended.ApprovalStatus != model.StoreStatusApproved {
		t.Fatalf("expected APPROVED after unsuspend, got %s", unsuspended.ApprovalStatus)
	}
}

func TestStateMachine_InvalidTransitions(t *testing.T) {
	repo := newMockStateRepo()
	svc := service.NewStoreService(repo)

	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-1",
		Role:   auth.RoleMerchant,
	})
	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   auth.RoleAdmin,
	})

	// 1. DRAFT store
	store, _ := svc.CreateStore(merchantCtx, "", dto.CreateStoreRequest{
		Name: "Draft Only Store",
	})

	// Try approving DRAFT store directly -> Error
	_, err := svc.ApproveStore(adminCtx, "", dto.ApproveStoreRequest{
		StoreID: store.ID,
	})
	if err == nil {
		t.Fatal("expected error approving DRAFT store without submission")
	}
	if appErrors.AsAppError(err).Code != appErrors.CodeUnprocessableEntity {
		t.Errorf("expected UNPROCESSABLE_ENTITY, got %s", appErrors.AsAppError(err).Code)
	}

	// Try rejecting DRAFT store directly -> Error
	_, err = svc.RejectStore(adminCtx, "", dto.RejectStoreRequest{
		StoreID: store.ID,
		Reason:  "Direct rejection attempt",
	})
	if err == nil {
		t.Fatal("expected error rejecting DRAFT store directly")
	}

	// Try publishing DRAFT store -> Error
	_, err = svc.PublishStore(merchantCtx, "", dto.PublishStoreRequest{
		StoreID: store.ID,
	})
	if err == nil {
		t.Fatal("expected error publishing unapproved DRAFT store")
	}

	// Try unsuspending DRAFT store -> Error
	_, err = svc.UnsuspendStore(adminCtx, "", dto.UnsuspendStoreRequest{
		StoreID: store.ID,
	})
	if err == nil {
		t.Fatal("expected error unsuspending DRAFT store")
	}

	// Submit store -> PENDING_APPROVAL
	_, _ = svc.SubmitStore(merchantCtx, "", dto.SubmitStoreRequest{StoreID: store.ID})

	// Try submitting PENDING_APPROVAL store again -> Error
	_, err = svc.SubmitStore(merchantCtx, "", dto.SubmitStoreRequest{StoreID: store.ID})
	if err == nil {
		t.Fatal("expected error submitting already submitted store")
	}

	// Approve store -> APPROVED
	_, _ = svc.ApproveStore(adminCtx, "", dto.ApproveStoreRequest{StoreID: store.ID})

	// Try submitting APPROVED store -> Error
	_, err = svc.SubmitStore(merchantCtx, "", dto.SubmitStoreRequest{StoreID: store.ID})
	if err == nil {
		t.Fatal("expected error submitting APPROVED store")
	}

	// Try approving APPROVED store again -> Error
	_, err = svc.ApproveStore(adminCtx, "", dto.ApproveStoreRequest{StoreID: store.ID})
	if err == nil {
		t.Fatal("expected error approving already APPROVED store")
	}
}
