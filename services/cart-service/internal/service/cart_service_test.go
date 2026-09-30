package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/service"
)

type mockCartRepo struct {
	carts map[string]*model.Cart
	cache map[string]*model.Cart
}

func newMockCartRepo() *mockCartRepo {
	return &mockCartRepo{
		carts: make(map[string]*model.Cart),
		cache: make(map[string]*model.Cart),
	}
}

func (m *mockCartRepo) GetCartByUserID(ctx context.Context, userID string) (*model.Cart, error) {
	for _, c := range m.carts {
		if c.UserID == userID {
			cp := deepCopyCart(c)
			return cp, nil
		}
	}
	return nil, repository.ErrCartNotFound
}

func (m *mockCartRepo) CreateCart(ctx context.Context, userID string) (*model.Cart, error) {
	cartID := fmt.Sprintf("cart-%s", userID)
	c := &model.Cart{
		ID:        cartID,
		UserID:    userID,
		Items:     make([]model.CartItem, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.carts[cartID] = c
	return deepCopyCart(c), nil
}

func (m *mockCartRepo) AddOrUpdateItem(ctx context.Context, cartID string, item *model.CartItem) (*model.Cart, error) {
	c, ok := m.carts[cartID]
	if !ok {
		return nil, repository.ErrCartNotFound
	}

	found := false
	for i := range c.Items {
		if c.Items[i].ProductID == item.ProductID && c.Items[i].VariantID == item.VariantID {
			c.Items[i].Quantity += item.Quantity
			c.Items[i].UnitPrice = item.UnitPrice
			c.Items[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}

	if !found {
		newItem := *item
		newItem.ID = fmt.Sprintf("item-%d", len(c.Items)+1)
		newItem.CartID = cartID
		newItem.CreatedAt = time.Now()
		newItem.UpdatedAt = time.Now()
		c.Items = append(c.Items, newItem)
	}

	c.CalculateTotal()
	c.UpdatedAt = time.Now()
	return deepCopyCart(c), nil
}

func (m *mockCartRepo) UpdateItemQuantity(ctx context.Context, cartID string, productID, variantID string, quantity int32) (*model.Cart, error) {
	c, ok := m.carts[cartID]
	if !ok {
		return nil, repository.ErrCartNotFound
	}

	found := false
	for i := range c.Items {
		if c.Items[i].ProductID == productID && c.Items[i].VariantID == variantID {
			c.Items[i].Quantity = quantity
			c.Items[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}

	if !found {
		return nil, repository.ErrItemNotFound
	}

	c.CalculateTotal()
	c.UpdatedAt = time.Now()
	return deepCopyCart(c), nil
}

func (m *mockCartRepo) RemoveItem(ctx context.Context, cartID string, productID, variantID string) (*model.Cart, error) {
	c, ok := m.carts[cartID]
	if !ok {
		return nil, repository.ErrCartNotFound
	}

	foundIdx := -1
	for i := range c.Items {
		if c.Items[i].ProductID == productID && c.Items[i].VariantID == variantID {
			foundIdx = i
			break
		}
	}

	if foundIdx == -1 {
		return nil, repository.ErrItemNotFound
	}

	c.Items = append(c.Items[:foundIdx], c.Items[foundIdx+1:]...)
	c.CalculateTotal()
	c.UpdatedAt = time.Now()
	return deepCopyCart(c), nil
}

func (m *mockCartRepo) ClearCart(ctx context.Context, cartID string) error {
	c, ok := m.carts[cartID]
	if !ok {
		return repository.ErrCartNotFound
	}
	c.Items = make([]model.CartItem, 0)
	c.TotalAmount = 0
	c.UpdatedAt = time.Now()
	return nil
}

func (m *mockCartRepo) GetCartFromCache(ctx context.Context, userID string) (*model.Cart, error) {
	c, ok := m.cache[userID]
	if !ok {
		return nil, nil
	}
	return deepCopyCart(c), nil
}

func (m *mockCartRepo) SetCartInCache(ctx context.Context, cart *model.Cart, ttl time.Duration) error {
	if cart == nil {
		return nil
	}
	m.cache[cart.UserID] = deepCopyCart(cart)
	return nil
}

func (m *mockCartRepo) DeleteCartFromCache(ctx context.Context, userID string) error {
	delete(m.cache, userID)
	return nil
}

func deepCopyCart(c *model.Cart) *model.Cart {
	if c == nil {
		return nil
	}
	cp := *c
	cp.Items = make([]model.CartItem, len(c.Items))
	copy(cp.Items, c.Items)
	return &cp
}

func TestCartService_Operations(t *testing.T) {
	ctx := context.Background()
	repo := newMockCartRepo()
	svc := service.NewCartService(repo, 3600, nil)

	userID := "user-123"
	prodID1 := "prod-001"
	varID1 := "var-001"
	storeID := "store-001"

	// 1. GetCart for non-existent cart creates a new cart
	cart, err := svc.GetCart(ctx, userID)
	if err != nil {
		t.Fatalf("expected GetCart to succeed, got %v", err)
	}
	if cart.UserID != userID {
		t.Errorf("expected UserID %s, got %s", userID, cart.UserID)
	}
	if len(cart.Items) != 0 {
		t.Errorf("expected empty items, got %d", len(cart.Items))
	}

	// 2. Add product/variant with quantity
	cart, err = svc.AddCartItem(ctx, dto.AddCartItemRequest{
		UserID:    userID,
		ProductID: prodID1,
		VariantID: varID1,
		StoreID:   storeID,
		UnitPrice: 49.99,
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("failed to add cart item: %v", err)
	}
	if len(cart.Items) != 1 {
		t.Fatalf("expected 1 item in cart, got %d", len(cart.Items))
	}
	if cart.Items[0].Quantity != 2 {
		t.Errorf("expected quantity 2, got %d", cart.Items[0].Quantity)
	}
	if cart.TotalAmount != 99.98 {
		t.Errorf("expected total amount 99.98, got %.2f", cart.TotalAmount)
	}

	// 3. Duplicate product/variant addition increments quantity consistently
	cart, err = svc.AddCartItem(ctx, dto.AddCartItemRequest{
		UserID:    userID,
		ProductID: prodID1,
		VariantID: varID1,
		StoreID:   storeID,
		UnitPrice: 49.99,
		Quantity:  3,
	})
	if err != nil {
		t.Fatalf("failed duplicate add cart item: %v", err)
	}
	if len(cart.Items) != 1 {
		t.Fatalf("expected 1 item in cart after duplicate addition, got %d", len(cart.Items))
	}
	if cart.Items[0].Quantity != 5 {
		t.Errorf("expected accumulated quantity 5, got %d", cart.Items[0].Quantity)
	}

	// 4. Quantity <= 0 is rejected on Add
	_, err = svc.AddCartItem(ctx, dto.AddCartItemRequest{
		UserID:    userID,
		ProductID: prodID1,
		Quantity:  0,
	})
	if err == nil {
		t.Fatal("expected addition with quantity 0 to be rejected")
	}
	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST error, got %v", err)
	}

	// 5. Update item quantity
	cart, err = svc.UpdateCartItem(ctx, dto.UpdateCartItemRequest{
		UserID:    userID,
		ProductID: prodID1,
		VariantID: varID1,
		Quantity:  10,
	})
	if err != nil {
		t.Fatalf("failed to update cart item quantity: %v", err)
	}
	if cart.Items[0].Quantity != 10 {
		t.Errorf("expected quantity 10, got %d", cart.Items[0].Quantity)
	}

	// 6. Quantity <= 0 is rejected on Update
	_, err = svc.UpdateCartItem(ctx, dto.UpdateCartItemRequest{
		UserID:    userID,
		ProductID: prodID1,
		VariantID: varID1,
		Quantity:  -1,
	})
	if err == nil {
		t.Fatal("expected update with negative quantity to be rejected")
	}

	// 7. Remove item
	cart, err = svc.RemoveCartItem(ctx, dto.RemoveCartItemRequest{
		UserID:    userID,
		ProductID: prodID1,
		VariantID: varID1,
	})
	if err != nil {
		t.Fatalf("failed to remove cart item: %v", err)
	}
	if len(cart.Items) != 0 {
		t.Errorf("expected cart to be empty after item removal, got %d", len(cart.Items))
	}

	// 8. Remove non-existent item fails with NOT_FOUND
	_, err = svc.RemoveCartItem(ctx, dto.RemoveCartItemRequest{
		UserID:    userID,
		ProductID: "non-existent",
	})
	if err == nil {
		t.Fatal("expected removing non-existent item to fail")
	}

	// 9. Add another item and test ClearCart
	_, _ = svc.AddCartItem(ctx, dto.AddCartItemRequest{
		UserID:    userID,
		ProductID: "prod-002",
		UnitPrice: 15.00,
		Quantity:  1,
	})
	cart, err = svc.ClearCart(ctx, userID)
	if err != nil {
		t.Fatalf("failed to clear cart: %v", err)
	}
	if len(cart.Items) != 0 || cart.TotalAmount != 0 {
		t.Errorf("expected cleared cart with 0 items and 0 total, got %d items, %.2f total", len(cart.Items), cart.TotalAmount)
	}
}

func TestCartService_RedisCaching(t *testing.T) {
	ctx := context.Background()
	repo := newMockCartRepo()
	svc := service.NewCartService(repo, 604800, nil)

	userID := "user-cache-test"
	cart, err := svc.AddCartItem(ctx, dto.AddCartItemRequest{
		UserID:    userID,
		ProductID: "p1",
		UnitPrice: 10.0,
		Quantity:  1,
	})
	if err != nil {
		t.Fatalf("failed to add item: %v", err)
	}

	// Verify cart stored in cache
	cached, err := repo.GetCartFromCache(ctx, userID)
	if err != nil || cached == nil {
		t.Fatalf("expected cart to be stored in cache, got cached=%v, err=%v", cached, err)
	}
	if cached.Items[0].ProductID != "p1" {
		t.Errorf("expected cached item p1, got %s", cached.Items[0].ProductID)
	}

	// GetCart should return cached value directly
	fetchedCart, err := svc.GetCart(ctx, userID)
	if err != nil || fetchedCart == nil {
		t.Fatalf("failed to get cart from cache: %v", err)
	}
	if fetchedCart.ID != cart.ID {
		t.Errorf("expected cart ID %s, got %s", cart.ID, fetchedCart.ID)
	}
}

func TestCartService_Validation(t *testing.T) {
	ctx := context.Background()
	repo := newMockCartRepo()
	svc := service.NewCartService(repo, 3600, nil)

	_, err := svc.GetCart(ctx, "")
	if err == nil || !strings.Contains(err.Error(), "user_id is required") {
		t.Errorf("expected user_id required error for empty GetCart, got %v", err)
	}

	_, err = svc.AddCartItem(ctx, dto.AddCartItemRequest{UserID: "u1", ProductID: "", Quantity: 1})
	if err == nil || !strings.Contains(err.Error(), "product_id is required") {
		t.Errorf("expected product_id required error for empty AddCartItem, got %v", err)
	}
}
