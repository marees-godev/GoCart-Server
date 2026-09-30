package handler_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockCartService struct {
	carts map[string]*model.Cart
}

func newMockCartService() *mockCartService {
	return &mockCartService{
		carts: make(map[string]*model.Cart),
	}
}

func (m *mockCartService) GetCart(ctx context.Context, userID string) (*model.Cart, error) {
	if userID == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}
	c, ok := m.carts[userID]
	if !ok {
		c = &model.Cart{
			ID:        fmt.Sprintf("cart-%s", userID),
			UserID:    userID,
			Items:     make([]model.CartItem, 0),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		m.carts[userID] = c
	}
	return c, nil
}

func (m *mockCartService) AddCartItem(ctx context.Context, req dto.AddCartItemRequest) (*model.Cart, error) {
	if req.UserID == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}
	if req.ProductID == "" {
		return nil, appErrors.BadRequest("product_id is required")
	}
	if req.Quantity <= 0 {
		return nil, appErrors.BadRequest("quantity must be greater than zero")
	}

	cart, _ := m.GetCart(ctx, req.UserID)
	found := false
	for i := range cart.Items {
		if cart.Items[i].ProductID == req.ProductID && cart.Items[i].VariantID == req.VariantID {
			cart.Items[i].Quantity += req.Quantity
			cart.Items[i].UnitPrice = req.UnitPrice
			found = true
			break
		}
	}
	if !found {
		cart.Items = append(cart.Items, model.CartItem{
			ID:        fmt.Sprintf("item-%d", len(cart.Items)+1),
			CartID:    cart.ID,
			ProductID: req.ProductID,
			VariantID: req.VariantID,
			StoreID:   req.StoreID,
			Quantity:  req.Quantity,
			UnitPrice: req.UnitPrice,
		})
	}
	cart.CalculateTotal()
	cart.UpdatedAt = time.Now()
	return cart, nil
}

func (m *mockCartService) UpdateCartItem(ctx context.Context, req dto.UpdateCartItemRequest) (*model.Cart, error) {
	if req.UserID == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}
	if req.ProductID == "" {
		return nil, appErrors.BadRequest("product_id is required")
	}
	if req.Quantity <= 0 {
		return nil, appErrors.BadRequest("quantity must be greater than zero")
	}

	cart, _ := m.GetCart(ctx, req.UserID)
	found := false
	for i := range cart.Items {
		if cart.Items[i].ProductID == req.ProductID && cart.Items[i].VariantID == req.VariantID {
			cart.Items[i].Quantity = req.Quantity
			found = true
			break
		}
	}
	if !found {
		return nil, appErrors.NotFound("item not found in cart")
	}
	cart.CalculateTotal()
	cart.UpdatedAt = time.Now()
	return cart, nil
}

func (m *mockCartService) RemoveCartItem(ctx context.Context, req dto.RemoveCartItemRequest) (*model.Cart, error) {
	if req.UserID == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}
	if req.ProductID == "" {
		return nil, appErrors.BadRequest("product_id is required")
	}

	cart, _ := m.GetCart(ctx, req.UserID)
	foundIdx := -1
	for i := range cart.Items {
		if cart.Items[i].ProductID == req.ProductID && cart.Items[i].VariantID == req.VariantID {
			foundIdx = i
			break
		}
	}
	if foundIdx == -1 {
		return nil, appErrors.NotFound("item not found in cart")
	}
	cart.Items = append(cart.Items[:foundIdx], cart.Items[foundIdx+1:]...)
	cart.CalculateTotal()
	cart.UpdatedAt = time.Now()
	return cart, nil
}

func (m *mockCartService) ClearCart(ctx context.Context, userID string) (*model.Cart, error) {
	if userID == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}
	cart, _ := m.GetCart(ctx, userID)
	cart.Items = make([]model.CartItem, 0)
	cart.CalculateTotal()
	cart.UpdatedAt = time.Now()
	return cart, nil
}

func TestCartGRPCHandler_UserAuthorization(t *testing.T) {
	svc := newMockCartService()
	h := handler.NewCartGRPCHandler(svc, nil)

	authUser := "user-alice"
	otherUser := "user-bob"

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: authUser,
		Role:   "CUSTOMER",
	})

	// 1. Authenticated user accesses another user's cart -> rejected with PermissionDenied
	_, err := h.GetCart(ctx, &cartpb.GetCartRequest{UserId: otherUser})
	if err == nil {
		t.Fatal("expected access to another user's cart to be rejected")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied status code, got %v", err)
	}

	// 2. Authenticated user accesses own cart -> succeeds
	res, err := h.GetCart(ctx, &cartpb.GetCartRequest{UserId: authUser})
	if err != nil {
		t.Fatalf("expected GetCart for own cart to succeed, got %v", err)
	}
	if res.Cart.UserId != authUser {
		t.Errorf("expected UserId %s, got %s", authUser, res.Cart.UserId)
	}

	// 3. User attempts to add item to another user's cart -> rejected
	_, err = h.AddCartItem(ctx, &cartpb.AddCartItemRequest{
		UserId:    otherUser,
		ProductId: "p1",
		Quantity:  1,
	})
	if err == nil {
		t.Fatal("expected AddCartItem for another user's cart to be rejected")
	}
	st, _ = status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}

func TestCartGRPCHandler_Operations(t *testing.T) {
	svc := newMockCartService()
	h := handler.NewCartGRPCHandler(svc, nil)

	userID := "user-1"
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: userID, Role: "CUSTOMER"})

	// AddCartItem
	addResp, err := h.AddCartItem(ctx, &cartpb.AddCartItemRequest{
		UserId:    userID,
		ProductId: "prod-1",
		VariantId: "var-1",
		StoreId:   "store-1",
		UnitPrice: 25.50,
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("failed AddCartItem: %v", err)
	}
	if len(addResp.Cart.Items) != 1 || addResp.Cart.TotalAmount != 51.00 {
		t.Errorf("unexpected cart after AddCartItem: %+v", addResp.Cart)
	}

	// AddCartItem
	addCartItemResp, err := h.AddCartItem(ctx, &cartpb.AddCartItemRequest{
		UserId:    userID,
		ProductId: "prod-2",
		UnitPrice: 10.00,
		Quantity:  1,
	})
	if err != nil {
		t.Fatalf("failed AddCartItem: %v", err)
	}
	if len(addCartItemResp.Cart.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(addCartItemResp.Cart.Items))
	}

	// Reject quantity <= 0
	_, err = h.AddCartItem(ctx, &cartpb.AddCartItemRequest{
		UserId:    userID,
		ProductId: "prod-3",
		Quantity:  0,
	})
	if err == nil {
		t.Fatal("expected InvalidArgument for quantity 0")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}

	// UpdateCartItem
	upResp, err := h.UpdateCartItem(ctx, &cartpb.UpdateCartItemRequest{
		UserId:    userID,
		ProductId: "prod-1",
		VariantId: "var-1",
		Quantity:  5,
	})
	if err != nil {
		t.Fatalf("failed UpdateCartItem: %v", err)
	}
	if upResp.Cart.Items[0].Quantity != 5 {
		t.Errorf("expected quantity 5, got %d", upResp.Cart.Items[0].Quantity)
	}

	// RemoveCartItem
	rmResp, err := h.RemoveCartItem(ctx, &cartpb.RemoveCartItemRequest{
		UserId:    userID,
		ProductId: "prod-1",
		VariantId: "var-1",
	})
	if err != nil {
		t.Fatalf("failed RemoveCartItem: %v", err)
	}
	if len(rmResp.Cart.Items) != 1 {
		t.Errorf("expected 1 item left, got %d", len(rmResp.Cart.Items))
	}

	// ClearCart
	clearResp, err := h.ClearCart(ctx, &cartpb.ClearCartRequest{
		UserId: userID,
	})
	if err != nil {
		t.Fatalf("failed ClearCart: %v", err)
	}
	if !clearResp.Success || len(clearResp.Cart.Items) != 0 {
		t.Errorf("expected clear cart success, got %+v", clearResp)
	}
}
