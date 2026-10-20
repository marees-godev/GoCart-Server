package resolvers

import (
	"context"
	"testing"

	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	grpcPkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockOrderServiceClient struct {
	orderpb.OrderServiceClient
	createResp *orderpb.CreateOrderResponse
	createErr  error
	getResp    *orderpb.GetOrderResponse
	getErr     error
	listResp   *orderpb.ListOrdersResponse
	listErr    error
	cancelResp *orderpb.CancelOrderResponse
	cancelErr  error
	updateResp *orderpb.UpdateOrderStatusResponse
	updateErr  error

	lastCreateReq *orderpb.CreateOrderRequest
	lastGetReq    *orderpb.GetOrderRequest
	lastListReq   *orderpb.ListOrdersRequest
	lastCancelReq *orderpb.CancelOrderRequest
}

func (m *mockOrderServiceClient) CreateOrder(ctx context.Context, in *orderpb.CreateOrderRequest, opts ...grpcPkg.CallOption) (*orderpb.CreateOrderResponse, error) {
	m.lastCreateReq = in
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.createResp, nil
}

func (m *mockOrderServiceClient) GetOrder(ctx context.Context, in *orderpb.GetOrderRequest, opts ...grpcPkg.CallOption) (*orderpb.GetOrderResponse, error) {
	m.lastGetReq = in
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.getResp, nil
}

func (m *mockOrderServiceClient) ListOrders(ctx context.Context, in *orderpb.ListOrdersRequest, opts ...grpcPkg.CallOption) (*orderpb.ListOrdersResponse, error) {
	m.lastListReq = in
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listResp, nil
}

func (m *mockOrderServiceClient) CancelOrder(ctx context.Context, in *orderpb.CancelOrderRequest, opts ...grpcPkg.CallOption) (*orderpb.CancelOrderResponse, error) {
	m.lastCancelReq = in
	if m.cancelErr != nil {
		return nil, m.cancelErr
	}
	return m.cancelResp, nil
}

func (m *mockOrderServiceClient) UpdateOrderStatus(ctx context.Context, in *orderpb.UpdateOrderStatusRequest, opts ...grpcPkg.CallOption) (*orderpb.UpdateOrderStatusResponse, error) {
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	return m.updateResp, nil
}

func setupTestOrderResolver(mockClient *mockOrderServiceClient) *Resolver {
	clients := &grpc.Clients{
		OrderClient: mockClient,
	}
	return NewResolver(clients, "1.0.0")
}

func TestOrderResolvers_CreateOrder_And_Checkout(t *testing.T) {
	userID := "usr-123"
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: userID,
		Role:   auth.RoleCustomer,
	})

	cartID := "cart-999"
	addr := "123 Market St, San Francisco, CA"
	fee := 10.0
	disc := 5.0

	mockClient := &mockOrderServiceClient{
		createResp: &orderpb.CreateOrderResponse{
			Order: &orderpb.Order{
				Id:              "ord-001",
				UserId:          userID,
				Status:          "CREATED",
				TotalAmount:     95.0,
				ShippingAddress: addr,
				Items: []*orderpb.OrderItem{
					{
						Id:          "item-1",
						ProductId:   "prod-1",
						Quantity:    2,
						UnitPrice:   50.0,
						TotalPrice:  90.0,
						ProductName: "Test Widget",
					},
				},
			},
		},
	}

	r := setupTestOrderResolver(mockClient)
	mutResolver := &mutationResolver{r}

	input := model.CreateOrderInput{
		CartID:          &cartID,
		ShippingAddress: &addr,
		ShippingFee:     &fee,
		DiscountAmount:  &disc,
	}

	// 1. Test createOrder mutation
	ord, err := mutResolver.CreateOrder(ctx, input)
	if err != nil {
		t.Fatalf("expected createOrder success, got: %v", err)
	}
	if ord.ID != "ord-001" || ord.UserID != userID || ord.TotalAmount != 95.0 {
		t.Errorf("unexpected order data: %+v", ord)
	}
	if mockClient.lastCreateReq.GetUserId() != userID {
		t.Errorf("expected gRPC user_id %s, got %s", userID, mockClient.lastCreateReq.GetUserId())
	}

	// 2. Test checkout mutation (alias)
	ord2, err := mutResolver.Checkout(ctx, input)
	if err != nil {
		t.Fatalf("expected checkout success, got: %v", err)
	}
	if ord2.ID != "ord-001" {
		t.Errorf("expected checkout to return created order, got %+v", ord2)
	}
}

func TestOrderResolvers_GetOrder(t *testing.T) {
	userID := "usr-123"
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: userID,
		Role:   auth.RoleCustomer,
	})

	mockClient := &mockOrderServiceClient{
		getResp: &orderpb.GetOrderResponse{
			Order: &orderpb.Order{
				Id:              "ord-001",
				UserId:          userID,
				Status:          "CONFIRMED",
				TotalAmount:     120.0,
				ShippingAddress: "123 Main St",
			},
		},
	}

	r := setupTestOrderResolver(mockClient)
	qResolver := &queryResolver{r}

	// 1. Success getOrder
	ord, err := qResolver.GetOrder(ctx, "ord-001")
	if err != nil {
		t.Fatalf("expected getOrder success, got %v", err)
	}
	if ord.ID != "ord-001" || ord.Status != "CONFIRMED" {
		t.Errorf("unexpected order data: %+v", ord)
	}
	if mockClient.lastGetReq.GetUserId() != userID {
		t.Errorf("expected user_id %s passed to gRPC, got %s", userID, mockClient.lastGetReq.GetUserId())
	}

	// 2. Not found error mapping
	mockClient.getErr = status.Error(codes.NotFound, "order not found")
	_, err = qResolver.GetOrder(ctx, "non-existent")
	if err == nil {
		t.Fatalf("expected not found error")
	}
	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeNotFound {
		t.Errorf("expected NOT_FOUND app error code, got %v", err)
	}

	// 3. Forbidden error mapping (trying to access someone else's order)
	mockClient.getErr = status.Error(codes.PermissionDenied, "you do not have permission to view this order")
	_, err = qResolver.GetOrder(ctx, "other-users-order")
	if err == nil {
		t.Fatalf("expected forbidden error")
	}
	appErr = appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN app error code, got %v", err)
	}
}

func TestOrderResolvers_ListMyOrders(t *testing.T) {
	userID := "usr-123"
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: userID,
		Role:   auth.RoleCustomer,
	})

	mockClient := &mockOrderServiceClient{
		listResp: &orderpb.ListOrdersResponse{
			Orders: []*orderpb.Order{
				{
					Id:          "ord-001",
					UserId:      userID,
					Status:      "CREATED",
					TotalAmount: 50.0,
				},
				{
					Id:          "ord-002",
					UserId:      userID,
					Status:      "DELIVERED",
					TotalAmount: 150.0,
				},
			},
			Total: 2,
		},
	}

	r := setupTestOrderResolver(mockClient)
	qResolver := &queryResolver{r}

	limit := 10
	offset := 0
	list, err := qResolver.ListMyOrders(ctx, &limit, &offset)
	if err != nil {
		t.Fatalf("expected listMyOrders success, got %v", err)
	}
	if list.Total != 2 || len(list.Orders) != 2 {
		t.Errorf("expected 2 orders, got total=%d len=%d", list.Total, len(list.Orders))
	}
	if mockClient.lastListReq.GetUserId() != userID {
		t.Errorf("expected gRPC called with user_id %s, got %s", userID, mockClient.lastListReq.GetUserId())
	}

	// Unauthenticated should fail
	_, err = qResolver.ListMyOrders(context.Background(), &limit, &offset)
	if err == nil {
		t.Fatalf("expected unauthenticated error for empty context")
	}
}

func TestOrderResolvers_CancelOrder(t *testing.T) {
	userID := "usr-123"
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: userID,
		Role:   auth.RoleCustomer,
	})

	mockClient := &mockOrderServiceClient{
		cancelResp: &orderpb.CancelOrderResponse{
			Order: &orderpb.Order{
				Id:          "ord-001",
				UserId:      userID,
				Status:      "CANCELLED",
				TotalAmount: 100.0,
			},
		},
	}

	r := setupTestOrderResolver(mockClient)
	mutResolver := &mutationResolver{r}

	reason := "Found cheaper price elsewhere"
	ord, err := mutResolver.CancelOrder(ctx, "ord-001", &reason)
	if err != nil {
		t.Fatalf("expected cancelOrder success, got %v", err)
	}
	if ord.Status != "CANCELLED" {
		t.Errorf("expected status CANCELLED, got %s", ord.Status)
	}
	if mockClient.lastCancelReq.GetReason() != reason {
		t.Errorf("expected reason %s forwarded, got %s", reason, mockClient.lastCancelReq.GetReason())
	}
	if mockClient.lastCancelReq.GetUserId() != userID {
		t.Errorf("expected user_id %s forwarded, got %s", userID, mockClient.lastCancelReq.GetUserId())
	}

	// Test cancellation rejection (PermissionDenied)
	mockClient.cancelErr = status.Error(codes.PermissionDenied, "you do not have permission to cancel this order")
	_, err = mutResolver.CancelOrder(ctx, "ord-different", nil)
	if err == nil {
		t.Fatalf("expected forbidden error on cancel")
	}
	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN app error, got %v", err)
	}

	// Test cancellation rejection (InvalidArgument / already shipped)
	mockClient.cancelErr = status.Error(codes.InvalidArgument, "cannot cancel order in SHIPPED status")
	_, err = mutResolver.CancelOrder(ctx, "ord-shipped", nil)
	if err == nil {
		t.Fatalf("expected bad request error on cancel")
	}
	appErr = appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST app error, got %v", err)
	}
}
