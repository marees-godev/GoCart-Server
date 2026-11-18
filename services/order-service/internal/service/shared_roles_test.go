package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/service"
)

func TestSharedRoles_OrderAccessMatrix(t *testing.T) {
	repo := newMockOrderRepo()
	orderSvc := service.NewOrderService(repo, nil, nil, nil, nil, 5, nil)

	customerID := uuid.New().String()
	merchantID := uuid.New().String()
	deliveryPartnerID := uuid.New().String()
	merchantStoreID := merchantID // merchant owns store

	// 1. Order placed by Customer at Merchant's Store
	custOrderID := uuid.New().String()
	repo.orders[custOrderID] = &model.Order{
		ID:          custOrderID,
		UserID:      customerID,
		StoreID:     merchantStoreID,
		Status:      model.OrderStatusCreated,
		TotalAmount: 100.0,
	}

	// 2. Personal shopping purchase placed by Merchant
	merchPurchaseID := uuid.New().String()
	repo.orders[merchPurchaseID] = &model.Order{
		ID:          merchPurchaseID,
		UserID:      merchantID,
		StoreID:     "other-store-abc",
		Status:      model.OrderStatusCreated,
		TotalAmount: 50.0,
	}

	// 3. Personal shopping purchase placed by Delivery Partner
	dpPurchaseID := uuid.New().String()
	repo.orders[dpPurchaseID] = &model.Order{
		ID:          dpPurchaseID,
		UserID:      deliveryPartnerID,
		StoreID:     "other-store-xyz",
		Status:      model.OrderStatusCreated,
		TotalAmount: 25.0,
	}

	t.Run("Customer can view own order, denied others", func(t *testing.T) {
		ctx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: customerID,
			Role:   auth.RoleCustomer,
		})
		ord, err := orderSvc.GetOrder(ctx, custOrderID)
		if err != nil {
			t.Fatalf("expected customer to view own order: %v", err)
		}
		if ord.ID != custOrderID {
			t.Errorf("expected %s, got %s", custOrderID, ord.ID)
		}

		// Denied viewing merchant purchase
		_, err = orderSvc.GetOrder(ctx, merchPurchaseID)
		if err == nil {
			t.Fatalf("customer should be forbidden from viewing another user's order")
		}
	})

	t.Run("Merchant can view own purchases as a shopper", func(t *testing.T) {
		ctx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: merchantID,
			Role:   auth.RoleMerchant,
		})
		ord, err := orderSvc.GetOrder(ctx, merchPurchaseID)
		if err != nil {
			t.Fatalf("expected merchant to view own purchase: %v", err)
		}
		if ord.ID != merchPurchaseID {
			t.Errorf("expected %s, got %s", merchPurchaseID, ord.ID)
		}
	})

	t.Run("Merchant can view customer orders for their own store", func(t *testing.T) {
		ctx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: merchantID,
			Role:   auth.RoleMerchant,
		})
		ord, err := orderSvc.GetOrder(ctx, custOrderID)
		if err != nil {
			t.Fatalf("expected merchant to view order for their store: %v", err)
		}
		if ord.ID != custOrderID {
			t.Errorf("expected %s, got %s", custOrderID, ord.ID)
		}
	})

	t.Run("Merchant cannot view orders for other stores they do not own", func(t *testing.T) {
		ctx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: merchantID,
			Role:   auth.RoleMerchant,
		})
		_, err := orderSvc.GetOrder(ctx, dpPurchaseID)
		if err == nil {
			t.Fatalf("merchant should be forbidden from viewing orders for other stores")
		}
	})

	t.Run("Delivery partner can view own purchases as a shopper", func(t *testing.T) {
		ctx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: deliveryPartnerID,
			Role:   auth.RoleDeliveryPartner,
		})
		ord, err := orderSvc.GetOrder(ctx, dpPurchaseID)
		if err != nil {
			t.Fatalf("expected delivery partner to view own purchase: %v", err)
		}
		if ord.ID != dpPurchaseID {
			t.Errorf("expected %s, got %s", dpPurchaseID, ord.ID)
		}

		// Denied viewing customer order
		_, err = orderSvc.GetOrder(ctx, custOrderID)
		if err == nil {
			t.Fatalf("delivery partner should be forbidden from viewing unrelated customer orders")
		}
	})

	t.Run("Admin can view any order", func(t *testing.T) {
		ctx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: "admin-master",
			Role:   auth.RoleAdmin,
		})
		for _, oID := range []string{custOrderID, merchPurchaseID, dpPurchaseID} {
			ord, err := orderSvc.GetOrder(ctx, oID)
			if err != nil {
				t.Fatalf("admin should be allowed to view any order %s: %v", oID, err)
			}
			if ord.ID != oID {
				t.Errorf("expected %s, got %s", oID, ord.ID)
			}
		}
	})
}
