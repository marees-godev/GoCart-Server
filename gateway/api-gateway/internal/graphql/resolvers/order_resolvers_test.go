package resolvers

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	notificationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/notification"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	paymentpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/payment"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/client"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	grpcPkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockDeliveryForOrderTest struct {
	deliverypb.DeliveryServiceClient
	feeResp *deliverypb.CalculateShippingFeeResponse
	lastReq *deliverypb.CalculateShippingFeeRequest
}

func (m *mockDeliveryForOrderTest) CalculateShippingFee(ctx context.Context, in *deliverypb.CalculateShippingFeeRequest, opts ...grpcPkg.CallOption) (*deliverypb.CalculateShippingFeeResponse, error) {
	m.lastReq = in
	if m.feeResp != nil {
		return m.feeResp, nil
	}
	return &deliverypb.CalculateShippingFeeResponse{ShippingFee: 55.0}, nil
}

func (m *mockDeliveryForOrderTest) CancelDelivery(ctx context.Context, in *deliverypb.CancelDeliveryRequest, opts ...grpcPkg.CallOption) (*deliverypb.CancelDeliveryResponse, error) {
	return &deliverypb.CancelDeliveryResponse{Success: true}, nil
}

type mockUserForOrderTest struct {
	userpb.UserServiceClient
	addr *userpb.Address
}

func (m *mockUserForOrderTest) GetUserAddress(ctx context.Context, in *userpb.GetUserAddressRequest, opts ...grpcPkg.CallOption) (*userpb.GetUserAddressResponse, error) {
	if m.addr != nil {
		return &userpb.GetUserAddressResponse{Address: m.addr}, nil
	}
	return nil, appErrors.NotFound("address not found")
}

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
		DeliveryPartnerID: "del-partner-default",
		CartID:            &cartID,
		ShippingAddress:   &addr,
		DiscountAmount:    &disc,
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

func TestOrderResolvers_CreateOrder_CalculatesShippingFeeWithAddressAndPartner(t *testing.T) {
	userID := "usr-123"
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: userID,
		Role:   auth.RoleCustomer,
	})

	mockOrder := &mockOrderServiceClient{
		createResp: &orderpb.CreateOrderResponse{
			Order: &orderpb.Order{
				Id:          "ord-fee-calc",
				UserId:      userID,
				Status:      "CREATED",
				ShippingFee: 55.0,
				TotalAmount: 155.0,
			},
		},
	}
	mockDelivery := &mockDeliveryForOrderTest{}
	mockUser := &mockUserForOrderTest{
		addr: &userpb.Address{
			AddressLine: "77 South Cross Road",
			City:        "Madurai",
			State:       "Tamil Nadu",
			PostalCode:  "625001",
		},
	}

	clients := &grpc.Clients{
		OrderClient:    mockOrder,
		DeliveryClient: mockDelivery,
		UserClient:     mockUser,
	}
	r := NewResolver(clients, "1.0.0")
	mutResolver := &mutationResolver{r}

	addrID := "addr-mdu"
	partnerID := "del-partner-1"
	// Client submits WITHOUT shippingFee in input
	input := model.CreateOrderInput{
		DeliveryPartnerID: partnerID,
		ShippingAddressID: &addrID,
	}

	ord, err := mutResolver.CreateOrder(ctx, input)
	if err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}
	if ord == nil {
		t.Fatal("expected non-nil order")
	}

	// Verify DeliveryClient was called with resolved address and partner
	if mockDelivery.lastReq == nil {
		t.Fatal("expected deliveryClient.CalculateShippingFee to have been called")
	}
	if mockDelivery.lastReq.PartnerId != "del-partner-1" {
		t.Errorf("expected partner id del-partner-1, got %s", mockDelivery.lastReq.PartnerId)
	}
	if mockDelivery.lastReq.DestinationCity != "Madurai" {
		t.Errorf("expected destination city Madurai, got %s", mockDelivery.lastReq.DestinationCity)
	}

	// Verify calculated shipping fee was passed to orderClient.CreateOrder
	if mockOrder.lastCreateReq == nil {
		t.Fatal("expected orderClient.CreateOrder to have been called")
	}
	if mockOrder.lastCreateReq.ShippingFee != 40.0 {
		t.Errorf("expected CreateOrder to be called with shippingFee 40.0, got %f", mockOrder.lastCreateReq.ShippingFee)
	}
}

func TestOrderResolvers_CreateOrder_MissingDeliveryPartnerID_Fails(t *testing.T) {
	userID := "usr-123"
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: userID,
		Role:   auth.RoleCustomer,
	})
	mockClient := &mockOrderServiceClient{}
	r := setupTestOrderResolver(mockClient)
	mutResolver := &mutationResolver{r}

	input := model.CreateOrderInput{
		DeliveryPartnerID: "   ",
	}

	_, err := mutResolver.CreateOrder(ctx, input)
	if err == nil {
		t.Fatal("expected error when deliveryPartnerId is empty, got nil")
	}
	if !strings.Contains(err.Error(), "deliveryPartnerId is required") {
		t.Errorf("expected 'deliveryPartnerId is required' error, got %v", err)
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

type mockNotifForOrderTest struct {
	notificationpb.NotificationServiceClient
	mu       sync.Mutex
	sentReqs []*notificationpb.SendNotificationRequest
}

func (m *mockNotifForOrderTest) SendNotification(ctx context.Context, in *notificationpb.SendNotificationRequest, opts ...grpcPkg.CallOption) (*notificationpb.SendNotificationResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentReqs = append(m.sentReqs, in)
	return &notificationpb.SendNotificationResponse{Success: true, NotificationId: "notif-test"}, nil
}

func TestOrderResolvers_SendNotificationOnCreateCancelUpdate(t *testing.T) {
	userID := "usr-notif-1"
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: userID,
		Role:   auth.RoleCustomer,
	})

	mockOrder := &mockOrderServiceClient{
		createResp: &orderpb.CreateOrderResponse{
			Order: &orderpb.Order{
				Id:          "ord-notif-1",
				UserId:      userID,
				Status:      "CREATED",
				TotalAmount: 250.00,
				Currency:    "INR",
			},
		},
		cancelResp: &orderpb.CancelOrderResponse{
			Order: &orderpb.Order{
				Id:          "ord-notif-1",
				UserId:      userID,
				Status:      "CANCELLED",
				TotalAmount: 250.00,
				Currency:    "INR",
			},
		},
		updateResp: &orderpb.UpdateOrderStatusResponse{
			Order: &orderpb.Order{
				Id:          "ord-notif-1",
				UserId:      userID,
				Status:      "CONFIRMED",
				TotalAmount: 250.00,
				Currency:    "INR",
			},
		},
	}
	mockNotif := &mockNotifForOrderTest{}
	clients := &grpc.Clients{
		OrderClient:        mockOrder,
		DeliveryClient:     &mockDeliveryForOrderTest{},
		NotificationClient: mockNotif,
	}
	r := NewResolver(clients, "1.0.0")
	mut := &mutationResolver{r}

	// 1. CreateOrder
	input := model.CreateOrderInput{
		DeliveryPartnerID: "partner-1",
	}
	ord, err := mut.CreateOrder(ctx, input)
	if err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}
	if ord == nil {
		t.Fatal("expected order response")
	}

	// Wait briefly for background notification dispatch
	time.Sleep(50 * time.Millisecond)

	mockNotif.mu.Lock()
	if len(mockNotif.sentReqs) == 0 {
		t.Errorf("expected SendNotification called on CreateOrder")
	} else {
		req := mockNotif.sentReqs[len(mockNotif.sentReqs)-1]
		if req.GetEventType() != "OrderCreated" {
			t.Errorf("expected event_type OrderCreated, got %s", req.GetEventType())
		}
		if req.GetUserId() != userID {
			t.Errorf("expected user_id %s, got %s", userID, req.GetUserId())
		}
	}
	mockNotif.mu.Unlock()

	// 2. CancelOrder
	reason := "Wrong address"
	_, err = mut.CancelOrder(ctx, "ord-notif-1", &reason)
	if err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mockNotif.mu.Lock()
	if len(mockNotif.sentReqs) < 2 {
		t.Errorf("expected at least 2 SendNotification calls, got %d", len(mockNotif.sentReqs))
	} else {
		req := mockNotif.sentReqs[len(mockNotif.sentReqs)-1]
		if req.GetEventType() != "OrderCancelled" {
			t.Errorf("expected event_type OrderCancelled, got %s", req.GetEventType())
		}
	}
	mockNotif.mu.Unlock()

	// 3. UpdateOrderStatus
	_, err = mut.UpdateOrderStatus(ctx, "ord-notif-1", "CONFIRMED")
	if err != nil {
		t.Fatalf("UpdateOrderStatus failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mockNotif.mu.Lock()
	if len(mockNotif.sentReqs) < 3 {
		t.Errorf("expected at least 3 SendNotification calls, got %d", len(mockNotif.sentReqs))
	} else {
		req := mockNotif.sentReqs[len(mockNotif.sentReqs)-1]
		if req.GetEventType() != "OrderConfirmed" {
			t.Errorf("expected event_type OrderConfirmed, got %s", req.GetEventType())
		}
	}
	mockNotif.mu.Unlock()
}

func TestMerchantCancelOrder_Restrictions(t *testing.T) {
	merchantID := "merchant-test-1"
	customerID := "customer-test-1"
	merchantCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: merchantID,
		Role:   "MERCHANT",
	})

	mockOrder := &mockOrderServiceClient{
		getResp: &orderpb.GetOrderResponse{
			Order: &orderpb.Order{
				Id:            "ord-conf-1",
				UserId:        customerID,
				Status:        "CONFIRMED",
				PaymentStatus: "PAID",
			},
		},
		cancelResp: &orderpb.CancelOrderResponse{
			Order: &orderpb.Order{
				Id:            "ord-conf-1",
				UserId:        customerID,
				Status:        "CANCELLED",
				PaymentStatus: "PAID",
			},
		},
	}

	clients := &grpc.Clients{
		OrderClient: mockOrder,
	}
	r := NewResolver(clients, "1.0.0")
	mut := r.Mutation()

	// 1. Merchant CANNOT cancel an already confirmed order (fails with FAILED_PRECONDITION)
	reason := "Cannot fulfill"
	_, err := mut.CancelOrder(merchantCtx, "ord-conf-1", &reason)
	if err == nil {
		t.Fatalf("expected cancelling confirmed order by merchant to fail")
	}
	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeFailedPrecondition {
		t.Fatalf("expected FAILED_PRECONDITION, got %v", err)
	}

	// 2. Merchant CANNOT cancel an unpaid order
	mockOrder.getResp = &orderpb.GetOrderResponse{
		Order: &orderpb.Order{
			Id:            "ord-unpaid-1",
			UserId:        customerID,
			Status:        "PENDING_PAYMENT",
			PaymentStatus: "PENDING",
		},
	}
	_, err = mut.CancelOrder(merchantCtx, "ord-unpaid-1", &reason)
	if err == nil {
		t.Fatalf("expected cancelling unpaid order by merchant to fail")
	}
	appErr = appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeFailedPrecondition {
		t.Fatalf("expected FAILED_PRECONDITION, got %v", err)
	}

	// 3. Merchant CAN cancel a paid order before confirmation
	mockOrder.getResp = &orderpb.GetOrderResponse{
		Order: &orderpb.Order{
			Id:            "ord-paid-1",
			UserId:        customerID,
			Status:        "PAID",
			PaymentStatus: "PAID",
		},
	}
	mockOrder.cancelResp = &orderpb.CancelOrderResponse{
		Order: &orderpb.Order{
			Id:            "ord-paid-1",
			UserId:        customerID,
			Status:        "CANCELLED",
			PaymentStatus: "PAID",
		},
	}
	res, err := mut.CancelOrder(merchantCtx, "ord-paid-1", &reason)
	if err != nil {
		t.Fatalf("expected merchant cancellation of paid order to succeed, got %v", err)
	}
	if res.Status != "CANCELLED" {
		t.Errorf("expected CANCELLED, got %s", res.Status)
	}

	// 4. Verify canCancel and merchantCanCancel flags in mapProtoOrderToGraphQL
	confirmedProto := &orderpb.Order{
		Id:            "ord-1",
		Status:        "CONFIRMED",
		PaymentStatus: "PAID",
	}
	mapped := mapProtoOrderToGraphQL(confirmedProto)
	if mapped.MerchantCanCancel == nil || *mapped.MerchantCanCancel != false {
		t.Errorf("expected MerchantCanCancel to be false for CONFIRMED order, got %v", mapped.MerchantCanCancel)
	}

	paidProto := &orderpb.Order{
		Id:            "ord-2",
		Status:        "PAID",
		PaymentStatus: "PAID",
	}
	mappedPaid := mapProtoOrderToGraphQL(paidProto)
	if mappedPaid.MerchantCanCancel == nil || *mappedPaid.MerchantCanCancel != true {
		t.Errorf("expected MerchantCanCancel to be true for PAID unconfirmed order, got %v", mappedPaid.MerchantCanCancel)
	}
}

func TestMapProtoOrderToGraphQL_PaymentStatusInference(t *testing.T) {
	// Orders confirmed or in fulfillment must display PAID
	for _, st := range []string{"PAID", "CONFIRMED", "PROCESSING", "SHIPPED", "OUT_FOR_DELIVERY", "DELIVERED", "COMPLETED"} {
		proto := &orderpb.Order{
			Id:            "ord-test-paid",
			Status:        st,
			PaymentStatus: "PENDING", // proto says pending, but status is terminal paid/fulfillment
		}
		mapped := mapProtoOrderToGraphQL(proto)
		if mapped.PaymentStatus == nil || *mapped.PaymentStatus != "PAID" {
			t.Errorf("expected payment status to be inferred as PAID for status %s, got %v", st, mapped.PaymentStatus)
		}
	}

	// Failed payment order
	failedProto := &orderpb.Order{
		Id:     "ord-failed",
		Status: "PAYMENT_FAILED",
	}
	mappedFailed := mapProtoOrderToGraphQL(failedProto)
	if mappedFailed.PaymentStatus == nil || *mappedFailed.PaymentStatus != "FAILED" {
		t.Errorf("expected payment status FAILED, got %v", mappedFailed.PaymentStatus)
	}

	// Refunded order
	refundedProto := &orderpb.Order{
		Id:     "ord-ref",
		Status: "REFUNDED",
	}
	mappedRef := mapProtoOrderToGraphQL(refundedProto)
	if mappedRef.PaymentStatus == nil || *mappedRef.PaymentStatus != "REFUNDED" {
		t.Errorf("expected payment status REFUNDED, got %v", mappedRef.PaymentStatus)
	}

	// Unpaid/Created order
	pendingProto := &orderpb.Order{
		Id:            "ord-pending",
		Status:        "CREATED",
		PaymentStatus: "PENDING",
	}
	mappedPending := mapProtoOrderToGraphQL(pendingProto)
	if mappedPending.PaymentStatus == nil || *mappedPending.PaymentStatus != "PENDING" {
		t.Errorf("expected payment status PENDING for unpaid created order, got %v", mappedPending.PaymentStatus)
	}
}

func TestEnrichOrderPaymentStatus_ReconcilesWithPaymentService(t *testing.T) {
	mockPay := &mockPaymentClientForResolvers{
		getByOrderIDResp: &paymentpb.GetPaymentResponse{
			Payment: &paymentpb.Payment{
				Id:      "pay-123",
				OrderId: "ORD-20261126-163033",
				Status:  "SUCCESS",
			},
		},
	}
	clientMgr := &client.ClientManager{
		PaymentClient: mockPay,
	}
	res := &Resolver{
		ClientMgr: clientMgr,
	}

	pStatus := "PENDING"
	orderNum := "ORD-20261126-163033"
	ord := &model.Order{
		ID:            "ord-uuid-1",
		OrderNumber:   &orderNum,
		Status:        "CREATED",
		PaymentStatus: &pStatus,
	}

	res.enrichOrderPaymentStatus(context.Background(), ord)

	if ord.PaymentStatus == nil || *ord.PaymentStatus != "PAID" {
		t.Errorf("expected payment status to be enriched to PAID from payment-service, got %v", ord.PaymentStatus)
	}
}
