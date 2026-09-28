package tests_test

import (
	"context"
	"testing"

	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/service"
)

func TestOwnership_CrossMerchantAccessDenied(t *testing.T) {
	repo := newMockStateRepo()
	svc := service.NewStoreService(repo)

	merchantA := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-A",
		Role:   auth.RoleMerchant,
	})

	merchantB := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-B",
		Role:   auth.RoleMerchant,
	})

	// Merchant A creates a store
	storeA, err := svc.CreateStore(merchantA, "", dto.CreateStoreRequest{
		Name: "Merchant A Store",
	})
	if err != nil {
		t.Fatalf("failed creating store A: %v", err)
	}

	// 1. Merchant B attempts Update on Merchant A's store
	newName := "Hacked Store Name"
	_, err = svc.UpdateStore(merchantB, "", storeA.ID, dto.UpdateStoreRequest{
		Name: &newName,
	})
	if err == nil {
		t.Error("expected forbidden error when Merchant B updates Merchant A's store")
	} else if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN, got %s", appErrors.AsAppError(err).Code)
	}

	// 2. Merchant B attempts Submit on Merchant A's store
	_, err = svc.SubmitStore(merchantB, "", dto.SubmitStoreRequest{
		StoreID: storeA.ID,
	})
	if err == nil {
		t.Error("expected forbidden error when Merchant B submits Merchant A's store")
	} else if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN, got %s", appErrors.AsAppError(err).Code)
	}

	// 3. Merchant B attempts KYC on Merchant A's store
	gstinVal := "27AAAAA0000A1Z5"
	_, err = svc.SubmitKYC(merchantB, "", dto.SubmitKYCRequest{
		StoreID:           storeA.ID,
		BankName:          "Hacker Bank",
		AccountNumber:     "999999999",
		AccountHolderName: "Hacker",
		GSTIN:             &gstinVal,
	})
	if err == nil {
		t.Error("expected forbidden error when Merchant B submits KYC for Merchant A's store")
	} else if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN, got %s", appErrors.AsAppError(err).Code)
	}

	// 4. Merchant B attempts Publish on Merchant A's store
	_, err = svc.PublishStore(merchantB, "", dto.PublishStoreRequest{
		StoreID: storeA.ID,
	})
	if err == nil {
		t.Error("expected forbidden error when Merchant B publishes Merchant A's store")
	} else if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN, got %s", appErrors.AsAppError(err).Code)
	}

	// 5. Merchant B attempts Unpublish on Merchant A's store
	_, err = svc.UnpublishStore(merchantB, "", dto.UnpublishStoreRequest{
		StoreID: storeA.ID,
	})
	if err == nil {
		t.Error("expected forbidden error when Merchant B unpublishes Merchant A's store")
	} else if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN, got %s", appErrors.AsAppError(err).Code)
	}

	// 6. Merchant B attempts Appeal on Merchant A's store
	_, _, err = svc.AppealStore(merchantB, "", dto.AppealStoreRequest{
		StoreID: storeA.ID,
		Reason:  "Appeal attempt by wrong merchant",
	})
	if err == nil {
		t.Error("expected forbidden error when Merchant B appeals Merchant A's store")
	} else if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN, got %s", appErrors.AsAppError(err).Code)
	}

	// 7. Merchant B attempts GetStoreAppeals for Merchant A's store
	_, err = svc.GetStoreAppeals(merchantB, "merchant-B", storeA.ID)
	if err == nil {
		t.Error("expected forbidden error when Merchant B views Merchant A's store appeals")
	} else if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN, got %s", appErrors.AsAppError(err).Code)
	}
}

func TestOwnership_MerchantCannotAdminOwnStore(t *testing.T) {
	repo := newMockStateRepo()
	svc := service.NewStoreService(repo)

	// User who has BOTH merchant store and admin privileges (conflict of interest test)
	merchantAdminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-admin-1",
		Role:   auth.RoleAdmin,
	})

	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-admin-1",
		Role:   auth.RoleMerchant,
	})

	// Create and submit store
	store, _ := svc.CreateStore(merchantCtx, "", dto.CreateStoreRequest{
		Name: "Conflict Store",
	})
	_, _ = svc.SubmitStore(merchantCtx, "", dto.SubmitStoreRequest{StoreID: store.ID})

	// Merchant tries to Approve their own store as admin -> Forbidden
	_, err := svc.ApproveStore(merchantAdminCtx, "", dto.ApproveStoreRequest{
		StoreID: store.ID,
	})
	if err == nil {
		t.Error("expected forbidden error when merchant tries to approve own store as admin")
	} else if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN, got %s", appErrors.AsAppError(err).Code)
	}

	// Merchant tries to Reject their own store as admin -> Forbidden
	_, err = svc.RejectStore(merchantAdminCtx, "", dto.RejectStoreRequest{
		StoreID: store.ID,
		Reason:  "Self rejection attempt",
	})
	if err == nil {
		t.Error("expected forbidden error when merchant tries to reject own store as admin")
	} else if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN, got %s", appErrors.AsAppError(err).Code)
	}

	// Merchant tries to Suspend their own store as admin -> Forbidden
	_, err = svc.SuspendStore(merchantAdminCtx, "", dto.SuspendStoreRequest{
		StoreID: store.ID,
		Reason:  "Self suspension attempt",
	})
	if err == nil {
		t.Error("expected forbidden error when merchant tries to suspend own store as admin")
	} else if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN, got %s", appErrors.AsAppError(err).Code)
	}
}

func TestOwnership_CustomerRoleForbidden(t *testing.T) {
	repo := newMockStateRepo()
	svc := service.NewStoreService(repo)

	customerCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "customer-100",
		Role:   auth.RoleCustomer,
	})

	// Customer tries to create store -> Forbidden
	_, err := svc.CreateStore(customerCtx, "", dto.CreateStoreRequest{
		Name: "Customer Store",
	})
	if err == nil {
		t.Error("expected forbidden error when customer creates store")
	} else if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN, got %s", appErrors.AsAppError(err).Code)
	}

	// Customer tries to approve store -> Forbidden
	_, err = svc.ApproveStore(customerCtx, "", dto.ApproveStoreRequest{
		StoreID: "store-100",
	})
	if err == nil {
		t.Error("expected forbidden error when customer approves store")
	} else if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN, got %s", appErrors.AsAppError(err).Code)
	}
}
