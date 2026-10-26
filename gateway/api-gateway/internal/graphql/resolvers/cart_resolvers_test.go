package resolvers

import (
	"context"
	"testing"

	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	grpcPkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockCartClient struct {
	cartpb.CartServiceClient
	addResp    *cartpb.AddCartItemResponse
	addErr     error
	getResp    *cartpb.GetCartResponse
	getErr     error
	removeResp *cartpb.RemoveCartItemResponse
	removeErr  error
	clearResp  *cartpb.ClearCartResponse
	clearErr   error

	lastAddReq    *cartpb.AddCartItemRequest
	lastGetReq    *cartpb.GetCartRequest
	lastRemoveReq *cartpb.RemoveCartItemRequest
	lastClearReq  *cartpb.ClearCartRequest
}

func (m *mockCartClient) AddCartItem(ctx context.Context, in *cartpb.AddCartItemRequest, opts ...grpcPkg.CallOption) (*cartpb.AddCartItemResponse, error) {
	m.lastAddReq = in
	if m.addErr != nil {
		return nil, m.addErr
	}
	return m.addResp, nil
}

func (m *mockCartClient) GetCart(ctx context.Context, in *cartpb.GetCartRequest, opts ...grpcPkg.CallOption) (*cartpb.GetCartResponse, error) {
	m.lastGetReq = in
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.getResp, nil
}

func (m *mockCartClient) RemoveCartItem(ctx context.Context, in *cartpb.RemoveCartItemRequest, opts ...grpcPkg.CallOption) (*cartpb.RemoveCartItemResponse, error) {
	m.lastRemoveReq = in
	if m.removeErr != nil {
		return nil, m.removeErr
	}
	return m.removeResp, nil
}

func (m *mockCartClient) ClearCart(ctx context.Context, in *cartpb.ClearCartRequest, opts ...grpcPkg.CallOption) (*cartpb.ClearCartResponse, error) {
	m.lastClearReq = in
	if m.clearErr != nil {
		return nil, m.clearErr
	}
	return m.clearResp, nil
}

func TestCartResolvers_AddToCart_Success(t *testing.T) {
	mockProd := &mockProductClient{
		getResp: &productpb.GetProductResponse{
			Product: &productpb.Product{
				Id:      "prod-101",
				Name:    "Gaming Mouse",
				Price:   49.99,
				StoreId: "store-55",
			},
		},
	}

	mockCart := &mockCartClient{
		addResp: &cartpb.AddCartItemResponse{
			Cart: &cartpb.Cart{
				Id:          "cart-1",
				UserId:      "jwt-user-42",
				TotalAmount: 99.98,
				Items: []*cartpb.CartItem{
					{
						Id:        "item-1",
						ProductId: "prod-101",
						Quantity:  2,
						UnitPrice: 49.99,
					},
				},
			},
		},
	}

	clients := &grpc.Clients{
		ProductClient: mockProd,
		CartClient:    mockCart,
	}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "jwt-user-42",
		Role:   "CUSTOMER",
	})

	cart, err := r.AddToCart(ctx, model.AddToCartInput{
		ProductID: "prod-101",
		Quantity:  2,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cart.UserID != "jwt-user-42" {
		t.Errorf("expected userId jwt-user-42, got %s", cart.UserID)
	}
	if cart.TotalAmount != 99.98 {
		t.Errorf("expected totalAmount 99.98, got %f", cart.TotalAmount)
	}
	if len(cart.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(cart.Items))
	}

	// Verify gRPC call forwarded the user ID from JWT and unit price from Product table
	if mockCart.lastAddReq == nil {
		t.Fatal("expected AddCartItem to be called on cart client")
	}
	if mockCart.lastAddReq.UserId != "jwt-user-42" {
		t.Errorf("expected AddCartItem UserId to be jwt-user-42, got %s", mockCart.lastAddReq.UserId)
	}
	if mockCart.lastAddReq.UnitPrice != 49.99 {
		t.Errorf("expected AddCartItem UnitPrice to be 49.99 from product table, got %f", mockCart.lastAddReq.UnitPrice)
	}
	if mockCart.lastAddReq.StoreId != "store-55" {
		t.Errorf("expected AddCartItem StoreId to be store-55, got %s", mockCart.lastAddReq.StoreId)
	}
	if mockCart.lastAddReq.Quantity != 2 {
		t.Errorf("expected AddCartItem Quantity to be 2, got %d", mockCart.lastAddReq.Quantity)
	}
}

func TestCartResolvers_AddToCart_Unauthenticated(t *testing.T) {
	clients := &grpc.Clients{}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	_, err := r.AddToCart(context.Background(), model.AddToCartInput{
		ProductID: "prod-101",
		Quantity:  1,
	})

	if err == nil {
		t.Fatal("expected unauthorized error when JWT context is missing")
	}
}

func TestCartResolvers_AddToCart_ProductNotFound(t *testing.T) {
	mockProd := &mockProductClient{
		getErr: status.Error(codes.NotFound, "product not found"),
	}
	clients := &grpc.Clients{
		ProductClient: mockProd,
		CartClient:    &mockCartClient{},
	}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "jwt-user-42",
		Role:   "CUSTOMER",
	})

	_, err := r.AddToCart(ctx, model.AddToCartInput{
		ProductID: "prod-non-existent",
		Quantity:  1,
	})

	if err == nil {
		t.Fatal("expected error when product not found")
	}
}

func TestCartResolvers_CartQuery_Success(t *testing.T) {
	mockCart := &mockCartClient{
		getResp: &cartpb.GetCartResponse{
			Cart: &cartpb.Cart{
				Id:          "cart-1",
				UserId:      "jwt-user-42",
				TotalAmount: 50.0,
				Items: []*cartpb.CartItem{
					{
						Id:        "item-1",
						ProductId: "prod-1",
						Quantity:  1,
						UnitPrice: 50.0,
					},
				},
			},
		},
	}

	clients := &grpc.Clients{CartClient: mockCart}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "jwt-user-42",
		Role:   "CUSTOMER",
	})

	cart, err := r.Cart(ctx, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cart.UserID != "jwt-user-42" {
		t.Errorf("expected userId jwt-user-42, got %s", cart.UserID)
	}
	if mockCart.lastGetReq.UserId != "jwt-user-42" {
		t.Errorf("expected gRPC request UserId to be jwt-user-42 from JWT, got %s", mockCart.lastGetReq.UserId)
	}
}

func TestCartResolvers_RemoveFromCart_Success(t *testing.T) {
	mockCart := &mockCartClient{
		removeResp: &cartpb.RemoveCartItemResponse{
			Cart: &cartpb.Cart{
				Id:          "cart-1",
				UserId:      "jwt-user-42",
				TotalAmount: 0,
				Items:       []*cartpb.CartItem{},
			},
		},
	}

	clients := &grpc.Clients{CartClient: mockCart}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "jwt-user-42",
		Role:   "CUSTOMER",
	})

	cart, err := r.RemoveFromCart(ctx, nil, "prod-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cart.UserID != "jwt-user-42" {
		t.Errorf("expected userId jwt-user-42, got %s", cart.UserID)
	}
	if mockCart.lastRemoveReq.UserId != "jwt-user-42" {
		t.Errorf("expected gRPC request UserId to be jwt-user-42 from JWT, got %s", mockCart.lastRemoveReq.UserId)
	}
	if mockCart.lastRemoveReq.ProductId != "prod-1" {
		t.Errorf("expected gRPC request ProductId to be prod-1, got %s", mockCart.lastRemoveReq.ProductId)
	}
}

func TestCartResolvers_ClearCart_Success(t *testing.T) {
	mockCart := &mockCartClient{
		clearResp: &cartpb.ClearCartResponse{
			Success: true,
		},
	}

	clients := &grpc.Clients{CartClient: mockCart}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "jwt-user-42",
		Role:   "CUSTOMER",
	})

	ok, err := r.ClearCart(ctx, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ok {
		t.Error("expected true from clearCart")
	}
	if mockCart.lastClearReq.UserId != "jwt-user-42" {
		t.Errorf("expected gRPC request UserId to be jwt-user-42 from JWT, got %s", mockCart.lastClearReq.UserId)
	}
}
