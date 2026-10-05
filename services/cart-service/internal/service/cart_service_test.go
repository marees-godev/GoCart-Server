package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	inventorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

type mockProductClient struct {
	products map[string]*productpb.Product
	err      error
}

func (m *mockProductClient) GetProduct(ctx context.Context, productID string) (*productpb.Product, error) {
	if m.err != nil {
		return nil, m.err
	}
	p, ok := m.products[productID]
	if !ok {
		return nil, status.Error(codes.NotFound, "product not found")
	}
	return p, nil
}

type mockInventoryClient struct {
	stocks map[string]*inventorypb.StockItem
	err    error
}

func (m *mockInventoryClient) GetStock(ctx context.Context, productID string) (*inventorypb.StockItem, error) {
	if m.err != nil {
		return nil, m.err
	}
	s, ok := m.stocks[productID]
	if !ok {
		return nil, status.Error(codes.NotFound, "stock not found")
	}
	return s, nil
}

func TestCartService_ValidateCart_SuccessAndPriceUpdate(t *testing.T) {
	ctx := context.Background()
	repo := newMockCartRepo()

	prodClient := &mockProductClient{
		products: map[string]*productpb.Product{
			"p1": {
				Id:       "p1",
				StoreId:  "store-1",
				Name:     "Product 1",
				Price:    29.99, // Authoritative price from Product Service
				Status:   "IN_STOCK",
			},
		},
	}
	invClient := &mockInventoryClient{
		stocks: map[string]*inventorypb.StockItem{
			"p1": {
				ProductId:         "p1",
				AvailableQuantity: 10,
			},
		},
	}

	svc := service.NewCartServiceWithClients(repo, prodClient, invClient, 5, 3600, nil)
	userID := "user-val-1"

	// Add item with client-supplied price 10.00 (stale/untrusted)
	_, err := svc.AddCartItem(ctx, dto.AddCartItemRequest{
		UserID:    userID,
		ProductID: "p1",
		StoreID:   "store-1",
		UnitPrice: 10.00,
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("failed to add item: %v", err)
	}

	valResp, err := svc.ValidateCart(ctx, userID)
	if err != nil {
		t.Fatalf("ValidateCart failed: %v", err)
	}

	if !valResp.IsValid {
		t.Errorf("expected cart to be valid, got errors: %+v", valResp.Errors)
	}

	// Verify price updated to authoritative 29.99
	if len(valResp.Cart.Items) != 1 || valResp.Cart.Items[0].UnitPrice != 29.99 {
		t.Errorf("expected unit_price to be updated to 29.99, got %v", valResp.Cart.Items[0].UnitPrice)
	}

	if valResp.Cart.TotalAmount != 59.98 {
		t.Errorf("expected total amount to be 59.98, got %v", valResp.Cart.TotalAmount)
	}

	if len(valResp.StoreGroups) != 1 || valResp.StoreGroups[0].StoreID != "store-1" {
		t.Errorf("expected 1 store group for store-1, got %+v", valResp.StoreGroups)
	}
}

func TestCartService_ValidateCart_OutOfStock(t *testing.T) {
	ctx := context.Background()
	repo := newMockCartRepo()

	prodClient := &mockProductClient{
		products: map[string]*productpb.Product{
			"p1": {
				Id:       "p1",
				Price:    50.0,
				Status:   "IN_STOCK",
			},
		},
	}
	invClient := &mockInventoryClient{
		stocks: map[string]*inventorypb.StockItem{
			"p1": {
				ProductId:         "p1",
				AvailableQuantity: 1, // Only 1 available, requested is 5
			},
		},
	}

	svc := service.NewCartServiceWithClients(repo, prodClient, invClient, 5, 3600, nil)
	userID := "user-stock-test"

	_, _ = svc.AddCartItem(ctx, dto.AddCartItemRequest{
		UserID:    userID,
		ProductID: "p1",
		UnitPrice: 50.0,
		Quantity:  5,
	})

	valResp, err := svc.ValidateCart(ctx, userID)
	if err != nil {
		t.Fatalf("ValidateCart failed: %v", err)
	}

	if valResp.IsValid {
		t.Fatal("expected cart validation to fail due to out-of-stock")
	}

	if len(valResp.Errors) == 0 || valResp.Errors[0].Code != dto.ErrCodeOutOfStock {
		t.Errorf("expected OUT_OF_STOCK error code %d, got %+v", dto.ErrCodeOutOfStock, valResp.Errors)
	}
}

func TestCartService_ValidateCart_DownstreamFailureControlledError(t *testing.T) {
	ctx := context.Background()
	repo := newMockCartRepo()

	prodClient := &mockProductClient{
		err: status.Error(codes.Unavailable, "product service unavailable"),
	}
	invClient := &mockInventoryClient{
		stocks: map[string]*inventorypb.StockItem{},
	}

	svc := service.NewCartServiceWithClients(repo, prodClient, invClient, 5, 3600, nil)
	userID := "user-downstream-err"

	_, _ = svc.AddCartItem(ctx, dto.AddCartItemRequest{
		UserID:    userID,
		ProductID: "p1",
		UnitPrice: 10.0,
		Quantity:  1,
	})

	_, err := svc.ValidateCart(ctx, userID)
	if err == nil {
		t.Fatal("expected controlled error on downstream service failure")
	}
}

func TestCartService_PrepareCheckout_MultiStoreGrouping(t *testing.T) {
	ctx := context.Background()
	repo := newMockCartRepo()

	prodClient := &mockProductClient{
		products: map[string]*productpb.Product{
			"p1": {Id: "p1", StoreId: "store-A", Price: 100.0, Status: "IN_STOCK"},
			"p2": {Id: "p2", StoreId: "store-B", Price: 50.0, Status: "IN_STOCK"},
		},
	}
	invClient := &mockInventoryClient{
		stocks: map[string]*inventorypb.StockItem{
			"p1": {ProductId: "p1", AvailableQuantity: 20},
			"p2": {ProductId: "p2", AvailableQuantity: 20},
		},
	}

	svc := service.NewCartServiceWithClients(repo, prodClient, invClient, 5, 3600, nil)
	userID := "user-multi-store"

	_, _ = svc.AddCartItem(ctx, dto.AddCartItemRequest{
		UserID:    userID,
		ProductID: "p1",
		StoreID:   "store-A",
		UnitPrice: 100.0,
		Quantity:  2,
	})
	_, _ = svc.AddCartItem(ctx, dto.AddCartItemRequest{
		UserID:    userID,
		ProductID: "p2",
		StoreID:   "store-B",
		UnitPrice: 50.0,
		Quantity:  1,
	})

	prepResp, err := svc.PrepareCheckout(ctx, dto.PrepareCheckoutRequest{
		UserID:          userID,
		ShippingAddress: "123 Main St, Tech City",
	})
	if err != nil {
		t.Fatalf("PrepareCheckout failed: %v", err)
	}

	if !prepResp.IsValid {
		t.Fatalf("expected PrepareCheckout to be valid, got errors: %+v", prepResp.Errors)
	}

	if prepResp.ParentOrderID == "" {
		t.Error("expected non-empty ParentOrderID")
	}

	if len(prepResp.Orders) != 2 {
		t.Fatalf("expected 2 store order payloads for multi-store cart, got %d", len(prepResp.Orders))
	}

	// Verify all store orders share the same ParentOrderID
	for _, o := range prepResp.Orders {
		if o.ParentOrderID != prepResp.ParentOrderID {
			t.Errorf("expected store order parent_order_id to be %s, got %s", prepResp.ParentOrderID, o.ParentOrderID)
		}
		if o.ShippingAddress != "123 Main St, Tech City" {
			t.Errorf("expected shipping address to be passed through, got %s", o.ShippingAddress)
		}
	}
}

