package resolvers

import (
	"context"
	"testing"

	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	grpcPkg "google.golang.org/grpc"
)

type mockDeliveryClient struct {
	deliverypb.DeliveryServiceClient
	lastCreateReq       *deliverypb.CreateDeliveryRequest
	lastUpdateReq       *deliverypb.UpdateDeliveryStatusRequest
	lastGetReq          *deliverypb.GetDeliveryRequest
	lastGetTrackingReq  *deliverypb.GetDeliveryByTrackingRequest
	lastGetOrderReq     *deliverypb.GetDeliveryByOrderRequest
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

func TestCreateDelivery_Success(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{Clients: clients},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "merchant-42",
		Role:   "MERCHANT",
	})

	input := model.CreateDeliveryInput{
		OrderID:         "ord-999",
		DeliveryPartner: "STANDARD",
		ShippingAddress: "200 Market St, San Francisco, CA 94105",
		ContactNumber:   "+1-415-555-0200",
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
	if deliveryMock.lastCreateReq.DeliveryPartner != "STANDARD" {
		t.Errorf("expected partner 'STANDARD', got %s", deliveryMock.lastCreateReq.DeliveryPartner)
	}
	if deliveryMock.lastCreateReq.ShippingAddress != "200 Market St, San Francisco, CA 94105" {
		t.Errorf("expected shipping address '200 Market St, San Francisco, CA 94105', got %s", deliveryMock.lastCreateReq.ShippingAddress)
	}
	if deliveryMock.lastCreateReq.ContactNumber != "+1-415-555-0200" {
		t.Errorf("expected contact number '+1-415-555-0200', got %s", deliveryMock.lastCreateReq.ContactNumber)
	}
}

func TestCreateDelivery_AdminSuccess(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{Clients: clients},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	})

	input := model.CreateDeliveryInput{
		OrderID:         "ord-100",
		DeliveryPartner: "DHL",
		ShippingAddress: "100 Pine St, San Francisco, CA 94111",
		ContactNumber:   "+1-415-555-0100",
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
		OrderID:         "ord-777",
		DeliveryPartner: "FEDEX",
		ShippingAddress: "",
		ContactNumber:   "+1-234-567-8900",
	}

	_, err := r.CreateDelivery(ctx, input)
	if err == nil {
		t.Fatal("expected error when shippingAddress is empty")
	}
}

func TestCreateDelivery_MissingContactNumber_Fails(t *testing.T) {
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
		OrderID:         "ord-777",
		DeliveryPartner: "FEDEX",
		ShippingAddress: "123 Main St",
		ContactNumber:   "",
	}

	_, err := r.CreateDelivery(ctx, input)
	if err == nil {
		t.Fatal("expected error when contactNumber is empty")
	}
}

func TestCreateDelivery_MissingDeliveryPartner_Fails(t *testing.T) {
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
		OrderID:         "ord-777",
		DeliveryPartner: "",
		ShippingAddress: "123 Main St",
		ContactNumber:   "+1-234-567-8900",
	}

	_, err := r.CreateDelivery(ctx, input)
	if err == nil {
		t.Fatal("expected error when delivery partner is empty")
	}
}

func TestCreateDelivery_Unauthorized(t *testing.T) {
	r := &mutationResolver{
		Resolver: &Resolver{Clients: &grpc.Clients{DeliveryClient: &mockDeliveryClient{}}},
	}

	_, err := r.CreateDelivery(context.Background(), model.CreateDeliveryInput{OrderID: "ord-1"})
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
