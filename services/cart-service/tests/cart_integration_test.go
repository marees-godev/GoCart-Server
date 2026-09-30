package tests_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type inMemoryCartRepo struct {
	carts map[string]*model.Cart
	cache map[string]*model.Cart
}

func newInMemoryCartRepo() *inMemoryCartRepo {
	return &inMemoryCartRepo{
		carts: make(map[string]*model.Cart),
		cache: make(map[string]*model.Cart),
	}
}

func (m *inMemoryCartRepo) GetCartByUserID(ctx context.Context, userID string) (*model.Cart, error) {
	for _, c := range m.carts {
		if c.UserID == userID {
			return deepCopyCart(c), nil
		}
	}
	return nil, repository.ErrCartNotFound
}

func (m *inMemoryCartRepo) CreateCart(ctx context.Context, userID string) (*model.Cart, error) {
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

func (m *inMemoryCartRepo) AddOrUpdateItem(ctx context.Context, cartID string, item *model.CartItem) (*model.Cart, error) {
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

func (m *inMemoryCartRepo) UpdateItemQuantity(ctx context.Context, cartID string, productID, variantID string, quantity int32) (*model.Cart, error) {
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

func (m *inMemoryCartRepo) RemoveItem(ctx context.Context, cartID string, productID, variantID string) (*model.Cart, error) {
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

func (m *inMemoryCartRepo) ClearCart(ctx context.Context, cartID string) error {
	c, ok := m.carts[cartID]
	if !ok {
		return repository.ErrCartNotFound
	}
	c.Items = make([]model.CartItem, 0)
	c.TotalAmount = 0
	c.UpdatedAt = time.Now()
	return nil
}

func (m *inMemoryCartRepo) GetCartFromCache(ctx context.Context, userID string) (*model.Cart, error) {
	c, ok := m.cache[userID]
	if !ok {
		return nil, nil
	}
	return deepCopyCart(c), nil
}

func (m *inMemoryCartRepo) SetCartInCache(ctx context.Context, cart *model.Cart, ttl time.Duration) error {
	if cart == nil {
		return nil
	}
	m.cache[cart.UserID] = deepCopyCart(cart)
	return nil
}

func (m *inMemoryCartRepo) DeleteCartFromCache(ctx context.Context, userID string) error {
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

func setupTestCartGRPCServer(t *testing.T) (cartpb.CartServiceClient, func()) {
	repo := newInMemoryCartRepo()
	svc := service.NewCartService(repo, 604800, nil)
	h := handler.NewCartGRPCHandler(svc, nil)

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcclient.UnaryServerInterceptor(),
		),
	)
	cartpb.RegisterCartServiceServer(server, h)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	go func() {
		_ = server.Serve(lis)
	}()

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial gRPC server: %v", err)
	}

	client := cartpb.NewCartServiceClient(conn)
	cleanup := func() {
		_ = conn.Close()
		server.Stop()
		_ = lis.Close()
	}

	return client, cleanup
}

func withUserAuth(ctx context.Context, userID string) context.Context {
	md := metadata.Pairs("x-user-id", userID, "x-user-role", auth.RoleCustomer)
	return metadata.NewOutgoingContext(ctx, md)
}

func TestCartService_E2EIntegration(t *testing.T) {
	client, cleanup := setupTestCartGRPCServer(t)
	defer cleanup()

	ctx := context.Background()
	userAlice := "alice-123"
	userBob := "bob-456"

	aliceCtx := withUserAuth(ctx, userAlice)
	bobCtx := withUserAuth(ctx, userBob)

	// 1. Retrieve cart for Alice (initially empty)
	getResp, err := client.GetCart(aliceCtx, &cartpb.GetCartRequest{UserId: userAlice})
	if err != nil {
		t.Fatalf("failed GetCart for Alice: %v", err)
	}
	if getResp.Cart.UserId != userAlice || len(getResp.Cart.Items) != 0 {
		t.Errorf("unexpected initial cart: %+v", getResp.Cart)
	}

	// 2. Prevent Alice from accessing Bob's cart
	_, err = client.GetCart(aliceCtx, &cartpb.GetCartRequest{UserId: userBob})
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied when Alice accesses Bob's cart, got %v", status.Code(err))
	}

	// 3. Add item to Alice's cart
	addResp, err := client.AddCartItem(aliceCtx, &cartpb.AddCartItemRequest{
		UserId:    userAlice,
		ProductId: "prod-mobile-1",
		VariantId: "var-red-128gb",
		StoreId:   "store-tech",
		UnitPrice: 699.99,
		Quantity:  1,
	})
	if err != nil {
		t.Fatalf("failed AddCartItem for Alice: %v", err)
	}
	if len(addResp.Cart.Items) != 1 || addResp.Cart.Items[0].ProductId != "prod-mobile-1" {
		t.Errorf("unexpected cart after AddCartItem: %+v", addResp.Cart)
	}

	// 4. Duplicate addition increments quantity
	addResp2, err := client.AddToCart(aliceCtx, &cartpb.AddToCartRequest{
		UserId:    userAlice,
		ProductId: "prod-mobile-1",
		VariantId: "var-red-128gb",
		StoreId:   "store-tech",
		UnitPrice: 699.99,
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("failed AddToCart duplicate for Alice: %v", err)
	}
	if len(addResp2.Cart.Items) != 1 || addResp2.Cart.Items[0].Quantity != 3 {
		t.Errorf("expected 1 item with quantity 3, got %+v", addResp2.Cart)
	}

	// 5. Reject quantity <= 0
	_, err = client.AddCartItem(aliceCtx, &cartpb.AddCartItemRequest{
		UserId:    userAlice,
		ProductId: "prod-mobile-2",
		Quantity:  0,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for quantity 0, got %v", status.Code(err))
	}

	// 6. Bob adds items to his own cart independently
	bobAddResp, err := client.AddCartItem(bobCtx, &cartpb.AddCartItemRequest{
		UserId:    userBob,
		ProductId: "prod-headphones",
		UnitPrice: 150.00,
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("failed AddCartItem for Bob: %v", err)
	}
	if bobAddResp.Cart.UserId != userBob || bobAddResp.Cart.TotalAmount != 300.00 {
		t.Errorf("unexpected cart for Bob: %+v", bobAddResp.Cart)
	}

	// 7. Update item quantity for Alice
	upResp, err := client.UpdateCartItem(aliceCtx, &cartpb.UpdateCartItemRequest{
		UserId:    userAlice,
		ProductId: "prod-mobile-1",
		VariantId: "var-red-128gb",
		Quantity:  5,
	})
	if err != nil {
		t.Fatalf("failed UpdateCartItem: %v", err)
	}
	if upResp.Cart.Items[0].Quantity != 5 {
		t.Errorf("expected quantity 5, got %d", upResp.Cart.Items[0].Quantity)
	}

	// 8. Remove item for Alice
	rmResp, err := client.RemoveCartItem(aliceCtx, &cartpb.RemoveCartItemRequest{
		UserId:    userAlice,
		ProductId: "prod-mobile-1",
		VariantId: "var-red-128gb",
	})
	if err != nil {
		t.Fatalf("failed RemoveCartItem: %v", err)
	}
	if len(rmResp.Cart.Items) != 0 {
		t.Errorf("expected empty cart for Alice, got %d items", len(rmResp.Cart.Items))
	}

	// 9. Clear cart for Bob
	clearResp, err := client.ClearCart(bobCtx, &cartpb.ClearCartRequest{UserId: userBob})
	if err != nil {
		t.Fatalf("failed ClearCart for Bob: %v", err)
	}
	if !clearResp.Success || len(clearResp.Cart.Items) != 0 {
		t.Errorf("expected cleared cart for Bob: %+v", clearResp)
	}
}
