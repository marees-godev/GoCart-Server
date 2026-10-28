package resolvers

import (
	"context"
	"testing"

	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	grpcPkg "google.golang.org/grpc"
)

type mockDeliveryClient struct {
	deliverypb.DeliveryServiceClient
	lastCreateReq       *deliverypb.CreateDeliveryRequest
	lastUpdateReq       *deliverypb.UpdateDeliveryStatusRequest
	lastGetReq          *deliverypb.GetDeliveryRequest
	lastGetTrackingReq  *deliverypb.GetDeliveryByTrackingRequest
	lastGetOrderReq     *deliverypb.GetDeliveryByOrderRequest
	lastCreatePartnerReq *deliverypb.CreatePartnerProfileRequest
}

func (m *mockDeliveryClient) GetPartnerProfile(ctx context.Context, in *deliverypb.GetPartnerProfileRequest, opts ...grpcPkg.CallOption) (*deliverypb.PartnerProfileResponse, error) {
	if in.PartnerId == "partner-uuid-1" {
		return &deliverypb.PartnerProfileResponse{
			Profile: &deliverypb.DeliveryPartnerProfile{
				Id:          "prof-1",
				PartnerId:   in.PartnerId,
				CompanyName: "Super Express Logistics",
				Email:       "contact@superexpress.com",
				IsActive:    true,
			},
		}, nil
	}
	return nil, appErrors.NotFound("partner profile not found")
}

func (m *mockDeliveryClient) CreatePartnerProfile(ctx context.Context, in *deliverypb.CreatePartnerProfileRequest, opts ...grpcPkg.CallOption) (*deliverypb.PartnerProfileResponse, error) {
	m.lastCreatePartnerReq = in
	return &deliverypb.PartnerProfileResponse{
		Profile: &deliverypb.DeliveryPartnerProfile{
			Id:            "prof-1",
			PartnerId:     in.PartnerId,
			CompanyName:   in.CompanyName,
			Email:         in.Email,
			ContactNumber: in.ContactNumber,
			IsActive:      true,
		},
	}, nil
}

func (m *mockDeliveryClient) CreateDelivery(ctx context.Context, in *deliverypb.CreateDeliveryRequest, opts ...grpcPkg.CallOption) (*deliverypb.CreateDeliveryResponse, error) {
	m.lastCreateReq = in
	return &deliverypb.CreateDeliveryResponse{
		Delivery: &deliverypb.Delivery{
			Id:              "del-123",
			OrderId:         in.OrderId,
			TrackingNumber:  "TRK-TEST-999",
			Status:          "CREATED",
			DeliveryPartner: in.DeliveryPartner,
			ShippingAddress: in.ShippingAddress,
			ContactNumber:   in.ContactNumber,
		},
	}, nil
}

func (m *mockDeliveryClient) UpdateDeliveryStatus(ctx context.Context, in *deliverypb.UpdateDeliveryStatusRequest, opts ...grpcPkg.CallOption) (*deliverypb.UpdateDeliveryStatusResponse, error) {
	m.lastUpdateReq = in
	return &deliverypb.UpdateDeliveryStatusResponse{
		Delivery: &deliverypb.Delivery{
			Id:     in.Id,
			Status: in.Status,
		},
	}, nil
}

func (m *mockDeliveryClient) GetDelivery(ctx context.Context, in *deliverypb.GetDeliveryRequest, opts ...grpcPkg.CallOption) (*deliverypb.GetDeliveryResponse, error) {
	m.lastGetReq = in
	return &deliverypb.GetDeliveryResponse{
		Delivery: &deliverypb.Delivery{
			Id:     in.Id,
			Status: "CREATED",
		},
	}, nil
}

func (m *mockDeliveryClient) GetDeliveryByTracking(ctx context.Context, in *deliverypb.GetDeliveryByTrackingRequest, opts ...grpcPkg.CallOption) (*deliverypb.GetDeliveryResponse, error) {
	m.lastGetTrackingReq = in
	return &deliverypb.GetDeliveryResponse{
		Delivery: &deliverypb.Delivery{
			Id:             "del-123",
			TrackingNumber: in.TrackingNumber,
			Status:         "IN_TRANSIT",
		},
	}, nil
}

func (m *mockDeliveryClient) GetDeliveryByOrder(ctx context.Context, in *deliverypb.GetDeliveryByOrderRequest, opts ...grpcPkg.CallOption) (*deliverypb.GetDeliveryResponse, error) {
	m.lastGetOrderReq = in
	return &deliverypb.GetDeliveryResponse{
		Delivery: &deliverypb.Delivery{
			Id:      "del-123",
			OrderId: in.OrderId,
			Status:  "DELIVERED",
		},
	}, nil
}

type mockOrderClientForDeliveryTest struct {
	orderpb.OrderServiceClient
	order *orderpb.Order
}

func (m *mockOrderClientForDeliveryTest) GetOrder(ctx context.Context, in *orderpb.GetOrderRequest, opts ...grpcPkg.CallOption) (*orderpb.GetOrderResponse, error) {
	if m.order != nil && m.order.Id == in.Id {
		return &orderpb.GetOrderResponse{Order: m.order}, nil
	}
	return nil, appErrors.NotFound("order not found")
}

type mockUserClientForDeliveryTest struct {
	userpb.UserServiceClient
	address *userpb.Address
	user    *userpb.User
}

func (m *mockUserClientForDeliveryTest) GetUserAddress(ctx context.Context, in *userpb.GetUserAddressRequest, opts ...grpcPkg.CallOption) (*userpb.GetUserAddressResponse, error) {
	if m.address != nil {
		return &userpb.GetUserAddressResponse{Address: m.address}, nil
	}
	return nil, appErrors.NotFound("address not found")
}

func (m *mockUserClientForDeliveryTest) GetUser(ctx context.Context, in *userpb.GetUserRequest, opts ...grpcPkg.CallOption) (*userpb.GetUserResponse, error) {
	if m.user != nil {
		return &userpb.GetUserResponse{User: m.user}, nil
	}
	return nil, appErrors.NotFound("user not found")
}

func strPtr(s string) *string {
	return &s
}

func TestCreateDelivery_Success(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	userMock := &mockUserClientForDeliveryTest{address: &userpb.Address{Id: "addr-1", AddressLine: "200 Market St, San Francisco, CA 94105"}}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{Clients: clients},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-42",
		Role:   "MERCHANT",
	})

	input := model.CreateDeliveryInput{
		OrderID:           "ord-999",
		DeliveryPartnerID: "partner-uuid-1",
		ShippingAddressID: "addr-1",
		ContactNumber:     strPtr("+1-415-555-0200"),
	}

	res, err := r.CreateDelivery(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID != "del-123" {
		t.Errorf("expected delivery ID del-123, got %s", res.ID)
	}

	if deliveryMock.lastCreateReq == nil {
		t.Fatal("expected DeliveryClient.CreateDelivery to be called")
	}
	if deliveryMock.lastCreateReq.OrderId != "ord-999" {
		t.Errorf("expected order ID 'ord-999', got %s", deliveryMock.lastCreateReq.OrderId)
	}
	if deliveryMock.lastCreateReq.DeliveryPartner != "Super Express Logistics" {
		t.Errorf("expected partner 'Super Express Logistics', got %s", deliveryMock.lastCreateReq.DeliveryPartner)
	}
	if deliveryMock.lastCreateReq.ShippingAddress != "200 Market St, San Francisco, CA 94105" {
		t.Errorf("expected shipping address '200 Market St, San Francisco, CA 94105', got %s", deliveryMock.lastCreateReq.ShippingAddress)
	}
	if deliveryMock.lastCreateReq.ContactNumber != "+1-415-555-0200" {
		t.Errorf("expected contact number '+1-415-555-0200', got %s", deliveryMock.lastCreateReq.ContactNumber)
	}
}

func TestCreateDelivery_WithDeliveryPartnerID_GetsCompanyName(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	userMock := &mockUserClientForDeliveryTest{address: &userpb.Address{Id: "addr-1", AddressLine: "200 Market St, San Francisco, CA 94105"}}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{Clients: clients},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-42",
		Role:   "MERCHANT",
	})

	input := model.CreateDeliveryInput{
		OrderID:           "ord-999",
		DeliveryPartnerID: "partner-uuid-1",
		ShippingAddressID: "addr-1",
		ContactNumber:     strPtr("+1-415-555-0200"),
	}

	res, err := r.CreateDelivery(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeliveryPartner != "Super Express Logistics" {
		t.Errorf("expected company name 'Super Express Logistics', got %s", res.DeliveryPartner)
	}
	if deliveryMock.lastCreateReq.DeliveryPartner != "Super Express Logistics" {
		t.Errorf("expected request DeliveryPartner 'Super Express Logistics', got %s", deliveryMock.lastCreateReq.DeliveryPartner)
	}
}

func TestCreateDelivery_ResolvesAddressAndPhoneFromOrder(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	orderMock := &mockOrderClientForDeliveryTest{
		order: &orderpb.Order{
			Id:              "ord-auto",
			UserId:          "user-cust-1",
			ShippingAddress: `{"street":"500 Howard St","city":"San Francisco","state":"CA","postal_code":"94105","contact_number":"+1-555-0199"}`,
		},
	}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		OrderClient:    orderMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{Clients: clients},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	})

	// Do not pass ShippingAddress or ContactNumber in input
	input := model.CreateDeliveryInput{
		OrderID:           "ord-auto",
		DeliveryPartnerID: "partner-uuid-1",
	}

	res, err := r.CreateDelivery(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeliveryPartner != "Super Express Logistics" {
		t.Errorf("expected company name 'Super Express Logistics', got %s", res.DeliveryPartner)
	}
	if deliveryMock.lastCreateReq.ContactNumber != "+1-555-0199" {
		t.Errorf("expected contact number '+1-555-0199', got %s", deliveryMock.lastCreateReq.ContactNumber)
	}
	if deliveryMock.lastCreateReq.ShippingAddress == "" {
		t.Error("expected shipping address to be resolved from order")
	}
}

func TestCreateDelivery_AdminSuccess(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	userMock := &mockUserClientForDeliveryTest{address: &userpb.Address{Id: "addr-100", AddressLine: "100 Pine St, San Francisco, CA 94111"}}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{Clients: clients},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	})

	input := model.CreateDeliveryInput{
		OrderID:           "ord-100",
		DeliveryPartnerID: "partner-uuid-1",
		ShippingAddressID: "addr-100",
		ContactNumber:     strPtr("+1-415-555-0100"),
	}

	res, err := r.CreateDelivery(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID != "del-123" {
		t.Errorf("expected delivery ID del-123, got %s", res.ID)
	}
}

func TestCreateDelivery_MissingShippingAddress_Fails(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{Clients: clients},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "usr-1",
		Role:   "CUSTOMER",
	})

	input := model.CreateDeliveryInput{
		OrderID:           "ord-777",
		DeliveryPartnerID: "partner-uuid-1",
		ShippingAddressID: "",
		ContactNumber:     strPtr("+1-234-567-8900"),
	}

	_, err := r.CreateDelivery(ctx, input)
	if err == nil {
		t.Fatal("expected error when shippingAddress cannot be resolved")
	}
}

func TestCreateDelivery_MissingContactNumber_Fails(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	userMock := &mockUserClientForDeliveryTest{address: &userpb.Address{Id: "addr-1", AddressLine: "123 Main St"}}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{Clients: clients},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "usr-1",
		Role:   "CUSTOMER",
	})

	input := model.CreateDeliveryInput{
		OrderID:           "ord-777",
		DeliveryPartnerID: "partner-uuid-1",
		ShippingAddressID: "addr-1",
		ContactNumber:     strPtr(""),
	}

	_, err := r.CreateDelivery(ctx, input)
	if err == nil {
		t.Fatal("expected error when contactNumber cannot be resolved")
	}
}

func TestCreateDelivery_MissingDeliveryPartnerID_Fails(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	userMock := &mockUserClientForDeliveryTest{address: &userpb.Address{Id: "addr-1", AddressLine: "123 Main St"}}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{Clients: clients},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "usr-1",
		Role:   "CUSTOMER",
	})

	input := model.CreateDeliveryInput{
		OrderID:           "ord-777",
		DeliveryPartnerID: "",
		ShippingAddressID: "addr-1",
		ContactNumber:     strPtr("+1-234-567-8900"),
	}

	_, err := r.CreateDelivery(ctx, input)
	if err == nil {
		t.Fatal("expected error when delivery partner id is empty")
	}
}

func TestCreateDelivery_Unauthorized(t *testing.T) {
	r := &mutationResolver{
		Resolver: &Resolver{Clients: &grpc.Clients{DeliveryClient: &mockDeliveryClient{}}},
	}

	_, err := r.CreateDelivery(context.Background(), model.CreateDeliveryInput{OrderID: "ord-1", DeliveryPartnerID: "partner-uuid-1", ShippingAddressID: "addr-1"})
	if err == nil {
		t.Fatal("expected unauthorized error when no user in context")
	}
}

func TestDeliveryQueriesAndStatusUpdate(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	clients := &grpc.Clients{DeliveryClient: deliveryMock}
	mResolver := &mutationResolver{Resolver: &Resolver{Clients: clients}}
	qResolver := &queryResolver{Resolver: &Resolver{Clients: clients}}

	ctx := context.Background()

	// Update status
	loc := "Depot A"
	upd, err := mResolver.UpdateDeliveryStatus(ctx, "del-1", model.DeliveryStatusInTransit, &loc)
	if err != nil || upd.Status != model.DeliveryStatusInTransit {
		t.Errorf("UpdateDeliveryStatus failed: %v", err)
	}

	// Get by ID
	d, err := qResolver.Delivery(ctx, "del-1")
	if err != nil || d.ID != "del-1" {
		t.Errorf("Delivery query failed: %v", err)
	}

	// Get by tracking
	dTrk, err := qResolver.DeliveryByTracking(ctx, "TRK-001")
	if err != nil || *dTrk.TrackingNumber != "TRK-001" {
		t.Errorf("DeliveryByTracking failed: %v", err)
	}

	// Get by order
	dOrd, err := qResolver.DeliveryByOrder(ctx, "ord-1")
	if err != nil || dOrd.OrderID != "ord-1" {
		t.Errorf("DeliveryByOrder failed: %v", err)
	}
}
