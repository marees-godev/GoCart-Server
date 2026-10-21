package handler_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockOrderService struct {
	createOrderFunc       func(ctx context.Context, input dto.CreateOrderInput) (*dto.CreateOrderResult, error)
	getOrderFunc          func(ctx context.Context, id string, userID ...string) (*model.Order, error)
	listOrdersFunc        func(ctx context.Context, input dto.ListOrdersInput) (*dto.ListOrdersResult, error)
	updateOrderStatusFunc func(ctx context.Context, id string, orderStatus model.OrderStatus, reason ...string) (*model.Order, error)
	cancelOrderFunc       func(ctx context.Context, id string, userID string, reason string) (*model.Order, error)
	getOrderStatusHistory func(ctx context.Context, orderID string) ([]*model.OrderStatusHistory, error)
}

func (m *mockOrderService) CreateOrder(ctx context.Context, input dto.CreateOrderInput) (*dto.CreateOrderResult, error) {
	if m.createOrderFunc != nil {
		return m.createOrderFunc(ctx, input)
	}
	return nil, nil
}

func (m *mockOrderService) GetOrder(ctx context.Context, id string, userID ...string) (*model.Order, error) {
	if m.getOrderFunc != nil {
		return m.getOrderFunc(ctx, id, userID...)
	}
	return nil, nil
}

func (m *mockOrderService) ListOrders(ctx context.Context, input dto.ListOrdersInput) (*dto.ListOrdersResult, error) {
	if m.listOrdersFunc != nil {
		return m.listOrdersFunc(ctx, input)
	}
	return nil, nil
}

func (m *mockOrderService) UpdateOrderStatus(ctx context.Context, id string, orderStatus model.OrderStatus, reason ...string) (*model.Order, error) {
	if m.updateOrderStatusFunc != nil {
		return m.updateOrderStatusFunc(ctx, id, orderStatus, reason...)
	}
	return nil, nil
}

func (m *mockOrderService) CancelOrder(ctx context.Context, id string, userID string, reason string) (*model.Order, error) {
	if m.cancelOrderFunc != nil {
		return m.cancelOrderFunc(ctx, id, userID, reason)
	}
	return nil, nil
}

func (m *mockOrderService) GetOrderStatusHistory(ctx context.Context, orderID string) ([]*model.OrderStatusHistory, error) {
	if m.getOrderStatusHistory != nil {
		return m.getOrderStatusHistory(ctx, orderID)
	}
	return nil, nil
}

func TestOrderGRPCHandler_CreateOrder(t *testing.T) {
	ctx := context.Background()

	parentID := uuid.New().String()
	orderID := uuid.New().String()
	userID := uuid.New().String()
	storeID := uuid.New().String()

	svc := &mockOrderService{
		createOrderFunc: func(ctx context.Context, input dto.CreateOrderInput) (*dto.CreateOrderResult, error) {
			if input.UserID == "" {
				return nil, appErrors.BadRequest("user_id is required")
			}
			ord := &model.Order{
				ID:            orderID,
				OrderNumber:   "ORD-20261014-111111",
				ParentOrderID: &parentID,
				UserID:        input.UserID,
				StoreID:       storeID,
				Status:        model.OrderStatusCreated,
				PaymentStatus: model.PaymentStatusPending,
				Subtotal:      100.00,
				TotalAmount:   110.00,
				CreatedAt:     time.Now().UTC(),
				UpdatedAt:     time.Now().UTC(),
				Items: []model.OrderItem{
					{
						ID:          uuid.New().String(),
						OrderID:     orderID,
						ProductID:   uuid.New().String(),
						StoreID:     storeID,
						SKU:         "SKU-1",
						ProductName: "Item 1",
						Quantity:    1,
						UnitPrice:   100.00,
						TotalPrice:  100.00,
					},
				},
			}
			return &dto.CreateOrderResult{
				PrimaryOrder:  ord,
				Orders:        []*model.Order{ord},
				ParentOrderID: parentID,
			}, nil
		},
	}

	h := handler.NewOrderGRPCHandler(svc, nil)

	// 1. Success
	resp, err := h.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		UserId:          userID,
		CartId:          "cart-1",
		ShippingAddress: "123 Test St",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.GetParentOrderId() != parentID {
		t.Errorf("expected parent_order_id %s, got %s", parentID, resp.GetParentOrderId())
	}
	if resp.GetOrder().GetId() != orderID {
		t.Errorf("expected order id %s, got %s", orderID, resp.GetOrder().GetId())
	}
	if len(resp.GetOrders()) != 1 {
		t.Errorf("expected 1 order, got %d", len(resp.GetOrders()))
	}

	// 2. Nil request
	_, err = h.CreateOrder(ctx, nil)
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for nil request, got %v", err)
	}

	// 3. Service error mapped to gRPC code
	svc.createOrderFunc = func(ctx context.Context, input dto.CreateOrderInput) (*dto.CreateOrderResult, error) {
		return nil, appErrors.Conflict("insufficient stock")
	}
	_, err = h.CreateOrder(ctx, &orderpb.CreateOrderRequest{UserId: userID})
	if status.Code(err) != codes.AlreadyExists {
		t.Errorf("expected AlreadyExists (Conflict) code, got %v", status.Code(err))
	}
}

func TestOrderGRPCHandler_GetOrder(t *testing.T) {
	ctx := context.Background()
	orderID := uuid.New().String()

	svc := &mockOrderService{
		getOrderFunc: func(ctx context.Context, id string, userID ...string) (*model.Order, error) {
			if id == "not-found" {
				return nil, appErrors.NotFound("order not found")
			}
			return &model.Order{
				ID:          orderID,
				OrderNumber: "ORD-123",
				Status:      model.OrderStatusCreated,
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			}, nil
		},
	}

	h := handler.NewOrderGRPCHandler(svc, nil)

	// 1. Success
	resp, err := h.GetOrder(ctx, &orderpb.GetOrderRequest{Id: orderID})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.GetOrder().GetId() != orderID {
		t.Errorf("expected order id %s, got %s", orderID, resp.GetOrder().GetId())
	}

	// 2. Missing id
	_, err = h.GetOrder(ctx, &orderpb.GetOrderRequest{Id: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", status.Code(err))
	}

	// 3. Not found
	_, err = h.GetOrder(ctx, &orderpb.GetOrderRequest{Id: "not-found"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound code, got %v", status.Code(err))
	}
}

func TestOrderGRPCHandler_ListOrders(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New().String()

	svc := &mockOrderService{
		listOrdersFunc: func(ctx context.Context, input dto.ListOrdersInput) (*dto.ListOrdersResult, error) {
			return &dto.ListOrdersResult{
				Orders: []*model.Order{
					{ID: "o-1", UserID: input.UserID, Status: model.OrderStatusCreated, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
					{ID: "o-2", UserID: input.UserID, Status: model.OrderStatusConfirmed, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
				},
				Total: 2,
			}, nil
		},
	}

	h := handler.NewOrderGRPCHandler(svc, nil)

	// 1. Success
	resp, err := h.ListOrders(ctx, &orderpb.ListOrdersRequest{UserId: userID, Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.GetTotal() != 2 || len(resp.GetOrders()) != 2 {
		t.Errorf("expected 2 orders and total 2, got %d and %d", len(resp.GetOrders()), resp.GetTotal())
	}

	// 2. Missing user_id
	_, err = h.ListOrders(ctx, &orderpb.ListOrdersRequest{UserId: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestOrderGRPCHandler_UpdateOrderStatus(t *testing.T) {
	ctx := context.Background()
	orderID := uuid.New().String()

	svc := &mockOrderService{
		updateOrderStatusFunc: func(ctx context.Context, id string, orderStatus model.OrderStatus, reason ...string) (*model.Order, error) {
			return &model.Order{
				ID:        id,
				Status:    orderStatus,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}, nil
		},
	}

	h := handler.NewOrderGRPCHandler(svc, nil)

	// 1. Success
	resp, err := h.UpdateOrderStatus(ctx, &orderpb.UpdateOrderStatusRequest{
		Id:     orderID,
		Status: "CONFIRMED",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.GetOrder().GetStatus() != "CONFIRMED" {
		t.Errorf("expected status CONFIRMED, got %s", resp.GetOrder().GetStatus())
	}

	// 2. Missing id
	_, err = h.UpdateOrderStatus(ctx, &orderpb.UpdateOrderStatusRequest{Id: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestOrderGRPCHandler_CancelOrder(t *testing.T) {
	ctx := context.Background()
	orderID := uuid.New().String()
	userID := uuid.New().String()

	svc := &mockOrderService{
		cancelOrderFunc: func(ctx context.Context, id string, uID string, reason string) (*model.Order, error) {
			if uID != userID {
				return nil, appErrors.Forbidden("you do not have permission to cancel this order")
			}
			return &model.Order{
				ID:        id,
				UserID:    uID,
				Status:    model.OrderStatusCancelled,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}, nil
		},
	}

	h := handler.NewOrderGRPCHandler(svc, nil)

	// 1. Successful cancellation
	resp, err := h.CancelOrder(ctx, &orderpb.CancelOrderRequest{
		Id:     orderID,
		UserId: userID,
		Reason: "Customer changed mind",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.GetOrder().GetStatus() != "CANCELLED" {
		t.Errorf("expected status CANCELLED, got %s", resp.GetOrder().GetStatus())
	}

	// 2. Missing order ID
	_, err = h.CancelOrder(ctx, &orderpb.CancelOrderRequest{
		Id:     "",
		UserId: userID,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", status.Code(err))
	}

	// 3. Unauthorized cancellation by another user
	_, err = h.CancelOrder(ctx, &orderpb.CancelOrderRequest{
		Id:     orderID,
		UserId: "different-user",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", status.Code(err))
	}
}
