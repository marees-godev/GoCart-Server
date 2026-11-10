package resolvers

import (
	"context"
	"strings"
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
	lastCalcReq         *deliverypb.CalculateShippingFeeRequest
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
			Id:                "del-123",
			OrderId:           in.OrderId,
			TrackingNumber:    "TRK-TEST-999",
			Status:            "CREATED",
			DeliveryPartner:   in.DeliveryPartner,
			ShippingAddress:   in.ShippingAddress,
			ContactNumber:     in.ContactNumber,
			ShippingAddressId: in.ShippingAddressId,
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
	if in.Id == "not-found" {
		return nil, appErrors.NotFound("delivery not found")
	}
	orderID := "ord-123"
	if in.Id == "del-no-order" {
		orderID = ""
	}
	return &deliverypb.GetDeliveryResponse{
		Delivery: &deliverypb.Delivery{
			Id:      in.Id,
			OrderId: orderID,
			Status:  "CREATED",
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

func (m *mockDeliveryClient) CancelDelivery(ctx context.Context, in *deliverypb.CancelDeliveryRequest, opts ...grpcPkg.CallOption) (*deliverypb.CancelDeliveryResponse, error) {
	if in.DeliveryId == "fail-id" {
		return nil, appErrors.UnprocessableEntity("delivery cannot be cancelled: parcel is already picked up")
	}
	return &deliverypb.CancelDeliveryResponse{
		Delivery: &deliverypb.Delivery{
			Id:     in.DeliveryId,
			Status: "CANCELLED",
		},
	}, nil
}

func (m *mockDeliveryClient) CalculateShippingFee(ctx context.Context, in *deliverypb.CalculateShippingFeeRequest, opts ...grpcPkg.CallOption) (*deliverypb.CalculateShippingFeeResponse, error) {
	m.lastCalcReq = in
	return &deliverypb.CalculateShippingFeeResponse{
		ShippingFee:      55.0,
		Currency:         "INR",
		Zone:             "TAMIL_NADU",
		TotalWeightGrams: in.WeightGrams,
		CalculationModel: "WEIGHT_BASED",
	}, nil
}

func (m *mockDeliveryClient) ActivatePartner(ctx context.Context, in *deliverypb.ActivatePartnerRequest, opts ...grpcPkg.CallOption) (*deliverypb.PartnerProfileResponse, error) {
	return &deliverypb.PartnerProfileResponse{
		Profile: &deliverypb.DeliveryPartnerProfile{
			Id:             "prof-1",
			PartnerId:      in.PartnerId,
			IsActive:       true,
			ApprovalStatus: "APPROVED",
		},
	}, nil
}

func (m *mockDeliveryClient) DeactivatePartner(ctx context.Context, in *deliverypb.DeactivatePartnerRequest, opts ...grpcPkg.CallOption) (*deliverypb.PartnerProfileResponse, error) {
	now := "2026-10-29T10:00:00Z"
	return &deliverypb.PartnerProfileResponse{
		Profile: &deliverypb.DeliveryPartnerProfile{
			Id:                 "prof-1",
			PartnerId:          in.PartnerId,
			IsActive:           false,
			DeactivatedAt:      now,
			DeactivationReason: in.Reason,
			ApprovalStatus:     "APPROVED",
		},
	}, nil
}

func (m *mockDeliveryClient) VerifyGSTIN(ctx context.Context, in *deliverypb.VerifyPartnerGSTINRequest, opts ...grpcPkg.CallOption) (*deliverypb.PartnerProfileResponse, error) {
	gstinVal := in.Gstin
	if gstinVal == "" {
		gstinVal = "29ABCDE1234F1Z5"
	}
	return &deliverypb.PartnerProfileResponse{
		Profile: &deliverypb.DeliveryPartnerProfile{
			Id:              "prof-1",
			PartnerId:       in.PartnerId,
			Gstin:           gstinVal,
			IsGstinVerified: true,
			ApprovalStatus:  "PENDING",
			IsActive:        true,
		},
	}, nil
}

func (m *mockDeliveryClient) ListAppealedPartners(ctx context.Context, in *deliverypb.ListAppealedPartnersRequest, opts ...grpcPkg.CallOption) (*deliverypb.ListAppealedPartnersResponse, error) {
	reason := "Valid appeal reason"
	return &deliverypb.ListAppealedPartnersResponse{
		Profiles: []*deliverypb.DeliveryPartnerProfile{
			{
				Id:             "prof-1",
				PartnerId:      "partner-uuid-1",
				CompanyName:    "Super Express Logistics",
				Email:          "contact@superexpress.com",
				ApprovalStatus: "APPEALED",
				AppealReason:   reason,
				IsActive:       true,
			},
		},
		Total: 1,
	}, nil
}

func (m *mockDeliveryClient) DeletePartnerProfile(ctx context.Context, in *deliverypb.DeletePartnerProfileRequest, opts ...grpcPkg.CallOption) (*deliverypb.DeletePartnerProfileResponse, error) {
	return &deliverypb.DeletePartnerProfileResponse{
		Success: true,
		Message: "account deleted successfully",
		Status:  "deleted",
	}, nil
}

func (m *mockDeliveryClient) ApprovePartner(ctx context.Context, in *deliverypb.ApprovePartnerRequest, opts ...grpcPkg.CallOption) (*deliverypb.PartnerProfileResponse, error) {
	return &deliverypb.PartnerProfileResponse{
		Profile: &deliverypb.DeliveryPartnerProfile{
			Id:              "prof-1",
			PartnerId:       in.PartnerId,
			ApprovalStatus:  "APPROVED",
			IsActive:        true,
			IsGstinVerified: true,
		},
	}, nil
}

func (m *mockDeliveryClient) RejectPartner(ctx context.Context, in *deliverypb.RejectPartnerRequest, opts ...grpcPkg.CallOption) (*deliverypb.PartnerProfileResponse, error) {
	return &deliverypb.PartnerProfileResponse{
		Profile: &deliverypb.DeliveryPartnerProfile{
			Id:              "prof-1",
			PartnerId:       in.PartnerId,
			ApprovalStatus:  "REJECTED",
			RejectionReason: in.Reason,
			IsActive:        true,
		},
	}, nil
}

func (m *mockDeliveryClient) AppealRejection(ctx context.Context, in *deliverypb.AppealRejectionRequest, opts ...grpcPkg.CallOption) (*deliverypb.PartnerProfileResponse, error) {
	return &deliverypb.PartnerProfileResponse{
		Profile: &deliverypb.DeliveryPartnerProfile{
			Id:             "prof-1",
			PartnerId:      in.PartnerId,
			ApprovalStatus: "APPEALED",
			AppealReason:   in.Reason,
			IsActive:       true,
		},
	}, nil
}

func (m *mockDeliveryClient) ListApprovedPartners(ctx context.Context, in *deliverypb.ListApprovedPartnersRequest, opts ...grpcPkg.CallOption) (*deliverypb.ListApprovedPartnersResponse, error) {
	return &deliverypb.ListApprovedPartnersResponse{
		Profiles: []*deliverypb.DeliveryPartnerProfile{
			{
				Id:             "prof-1",
				PartnerId:      "partner-uuid-1",
				CompanyName:    "Super Express Logistics",
				Email:          "contact@superexpress.com",
				ApprovalStatus: "APPROVED",
				IsActive:       true,
			},
		},
		Total: 1,
	}, nil
}

type mockOrderClientForDeliveryTest struct {
	orderpb.OrderServiceClient
	resp  *orderpb.GetOrderResponse
	err   error
	order *orderpb.Order
}

func (m *mockOrderClientForDeliveryTest) GetOrder(ctx context.Context, in *orderpb.GetOrderRequest, opts ...grpcPkg.CallOption) (*orderpb.GetOrderResponse, error) {
	if m.err != nil { return nil, m.err }
	if m.resp != nil { return m.resp, nil }
	if m.order != nil && m.order.Id == in.Id {
		return &orderpb.GetOrderResponse{Order: m.order}, nil
	}
	return nil, appErrors.NotFound("order not found")
}

func (m *mockOrderClientForDeliveryTest) CancelOrder(ctx context.Context, in *orderpb.CancelOrderRequest, opts ...grpcPkg.CallOption) (*orderpb.CancelOrderResponse, error) {
	if m.err != nil { return nil, m.err }
	return &orderpb.CancelOrderResponse{Order: m.order}, nil
}

func (m *mockOrderClientForDeliveryTest) UpdateOrderStatus(ctx context.Context, in *orderpb.UpdateOrderStatusRequest, opts ...grpcPkg.CallOption) (*orderpb.UpdateOrderStatusResponse, error) {
	if m.err != nil { return nil, m.err }
	return &orderpb.UpdateOrderStatusResponse{Order: m.order}, nil
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

func deliveryStrPtr(s string) *string {
	return &s
}

func TestCreateDelivery_Success(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	userMock := &mockUserClientForDeliveryTest{address: &userpb.Address{Id: "addr-1", AddressLine: "200 Market St, San Francisco, CA 94105"}}
	orderMock := &mockOrderClientForDeliveryTest{order: &orderpb.Order{Id: "ord-999", PaymentStatus: "PAID"}}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		UserClient: userMock,
		OrderClient: orderMock,
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
		ContactNumber:     deliveryStrPtr("+1-415-555-0200"),
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
	if deliveryMock.lastCreateReq.ShippingAddressId != "addr-1" {
		t.Errorf("expected shipping address id 'addr-1', got %s", deliveryMock.lastCreateReq.ShippingAddressId)
	}
	if res.ShippingAddressID == nil || *res.ShippingAddressID != "addr-1" {
		t.Errorf("expected res.ShippingAddressID to be 'addr-1'")
	}
}

func TestCreateDelivery_WithDeliveryPartnerID_GetsCompanyName(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	userMock := &mockUserClientForDeliveryTest{address: &userpb.Address{Id: "addr-1", AddressLine: "200 Market St, San Francisco, CA 94105"}}
	orderMock := &mockOrderClientForDeliveryTest{order: &orderpb.Order{Id: "ord-999", PaymentStatus: "PAID"}}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		UserClient: userMock,
		OrderClient: orderMock,
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
		ContactNumber:     deliveryStrPtr("+1-415-555-0200"),
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
			Id:                "ord-auto",
			UserId:            "user-cust-1",
			ShippingAddressId: "addr-auto-1",
			ShippingAddress:   `{"street":"500 Howard St","city":"San Francisco","state":"CA","postal_code":"94105","contact_number":"+1-555-0199"}`,
			PaymentStatus:     "PAID",
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

func TestCreateDelivery_OrderPaymentStatusNotPaid_Fails(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	orderMock := &mockOrderClientForDeliveryTest{
		order: &orderpb.Order{
			Id:                "ord-pending-payment",
			UserId:            "user-cust-1",
			ShippingAddressId: "addr-auto-1",
			ShippingAddress:   `{"street":"500 Howard St","city":"San Francisco","state":"CA","postal_code":"94105","contact_number":"+1-555-0199"}`,
			PaymentStatus:     "PENDING",
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

	input := model.CreateDeliveryInput{
		OrderID:           "ord-pending-payment",
		DeliveryPartnerID: "partner-uuid-1",
	}

	_, err := r.CreateDelivery(ctx, input)
	if err == nil {
		t.Fatal("expected error when order payment_status is not PAID, got nil")
	}
	if !strings.Contains(err.Error(), "payment_status must be PAID") {
		t.Errorf("expected 'payment_status must be PAID' error, got %v", err)
	}
}

func TestCreateDelivery_AdminSuccess(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	userMock := &mockUserClientForDeliveryTest{address: &userpb.Address{Id: "addr-100", AddressLine: "100 Pine St, San Francisco, CA 94111"}}
	orderMock := &mockOrderClientForDeliveryTest{order: &orderpb.Order{Id: "ord-100", PaymentStatus: "PAID"}}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		UserClient: userMock,
		OrderClient: orderMock,
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
		ContactNumber:     deliveryStrPtr("+1-415-555-0100"),
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
	orderMock := &mockOrderClientForDeliveryTest{order: &orderpb.Order{Id: "ord-777", PaymentStatus: "PAID"}}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		OrderClient: orderMock,
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
		ContactNumber:     deliveryStrPtr("+1-234-567-8900"),
	}

	_, err := r.CreateDelivery(ctx, input)
	if err == nil {
		t.Fatal("expected error when shippingAddress cannot be resolved")
	}
}

func TestCreateDelivery_MissingContactNumber_Fails(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	userMock := &mockUserClientForDeliveryTest{address: &userpb.Address{Id: "addr-1", AddressLine: "123 Main St"}}
	orderMock := &mockOrderClientForDeliveryTest{order: &orderpb.Order{Id: "ord-777", PaymentStatus: "PAID"}}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		UserClient: userMock,
		OrderClient: orderMock,
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
		ContactNumber:     deliveryStrPtr(""),
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
		ContactNumber:     deliveryStrPtr("+1-234-567-8900"),
	}

	_, err := r.CreateDelivery(ctx, input)
	if err == nil {
		t.Fatal("expected error when delivery partner id is empty")
	}
}

func TestCreateDelivery_MissingShippingAddressID_Fails(t *testing.T) {
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
		ContactNumber:     deliveryStrPtr("+1-234-567-8900"),
	}

	_, err := r.CreateDelivery(ctx, input)
	if err == nil {
		t.Fatal("expected error when shipping address id is empty")
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

func TestDeliveryPartner_ApprovalAndAccountMutations(t *testing.T) {
	clients := &grpc.Clients{
		DeliveryClient: &mockDeliveryClient{},
	}
	mResolver := &mutationResolver{Resolver: &Resolver{Clients: clients}}
	qResolver := &queryResolver{Resolver: &Resolver{Clients: clients}}

	partnerCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:    "partner-uuid-1",
		Role:      auth.RoleDeliveryPartner,
		PartnerID: "partner-uuid-1",
	})
	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-user-1",
		Role:   auth.RoleAdmin,
	})
	customerCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "customer-user-1",
		Role:   "CUSTOMER",
	})

	t.Run("VerifyDeliveryPartnerGSTIN", func(t *testing.T) {
		targetPartnerID := "partner-uuid-1"

		// Customer cannot verify GSTIN
		_, err := mResolver.VerifyDeliveryPartnerGstin(customerCtx, targetPartnerID)
		if err == nil {
			t.Fatal("expected customer GSTIN verification to be forbidden, got nil")
		}

		// Partner cannot verify GSTIN (only admin can)
		_, err = mResolver.VerifyDeliveryPartnerGstin(partnerCtx, targetPartnerID)
		if err == nil {
			t.Fatal("expected partner GSTIN verification to be forbidden, got nil")
		}

		// Empty partner ID fails
		_, err = mResolver.VerifyDeliveryPartnerGstin(adminCtx, "   ")
		if err == nil {
			t.Fatal("expected empty partner ID to fail, got nil")
		}

		// Admin succeeds
		res, err := mResolver.VerifyDeliveryPartnerGstin(adminCtx, targetPartnerID)
		if err != nil {
			t.Fatalf("expected admin verify GSTIN to succeed, got %v", err)
		}
		if !res.IsGstinVerified {
			t.Fatal("expected is_gstin_verified to be true")
		}
		if res.Gstin == nil || *res.Gstin != "29ABCDE1234F1Z5" {
			t.Fatalf("expected gstin to be 29ABCDE1234F1Z5, got %v", res.Gstin)
		}
	})

	t.Run("ActivateDeliveryPartner", func(t *testing.T) {
		// Admin cannot activate partner (owner only)
		_, err := mResolver.ActivateDeliveryPartner(adminCtx)
		if err == nil {
			t.Fatal("expected admin activation to be forbidden, got nil")
		}

		// Customer cannot activate partner
		_, err = mResolver.ActivateDeliveryPartner(customerCtx)
		if err == nil {
			t.Fatal("expected customer activation to be forbidden, got nil")
		}

		res, err := mResolver.ActivateDeliveryPartner(partnerCtx)
		if err != nil {
			t.Fatalf("expected activation to succeed, got %v", err)
		}
		if !res.Success {
			t.Fatal("expected success to be true")
		}
		if res.Status != "active" {
			t.Fatalf("expected status 'active', got %s", res.Status)
		}
		if res.Message != "account reactivated successfully" {
			t.Fatalf("expected message 'account reactivated successfully', got %s", res.Message)
		}

		reactRes, err := mResolver.ReactivateDeliveryPartner(partnerCtx)
		if err != nil {
			t.Fatalf("expected reactivate to succeed, got %v", err)
		}
		if !reactRes.Success || reactRes.Status != "active" {
			t.Fatalf("expected reactivate success and active status, got %+v", reactRes)
		}
	})

	t.Run("DeactivateDeliveryPartner", func(t *testing.T) {
		// Admin cannot deactivate partner (owner only)
		_, err := mResolver.DeactivateDeliveryPartner(adminCtx, "Admin shutdown")
		if err == nil {
			t.Fatal("expected admin deactivation to be forbidden, got nil")
		}

		// Customer cannot deactivate partner
		_, err = mResolver.DeactivateDeliveryPartner(customerCtx, "Customer shutdown")
		if err == nil {
			t.Fatal("expected customer deactivation to be forbidden, got nil")
		}

		// Empty reason fails
		_, err = mResolver.DeactivateDeliveryPartner(partnerCtx, "   ")
		if err == nil {
			t.Fatal("expected empty reason to fail, got nil")
		}

		res, err := mResolver.DeactivateDeliveryPartner(partnerCtx, "Business reorganization")
		if err != nil {
			t.Fatalf("expected deactivation to succeed, got %v", err)
		}
		if !res.Success {
			t.Fatal("expected success to be true")
		}
		if res.Status != "deactivated" {
			t.Fatalf("expected status 'deactivated', got %s", res.Status)
		}
		if res.Message != "account deactivated successfully" {
			t.Fatalf("expected message 'account deactivated successfully', got %s", res.Message)
		}
	})

	t.Run("DeleteDeliveryPartner owner check", func(t *testing.T) {
		// Customer cannot delete partner
		_, err := mResolver.DeleteDeliveryPartner(customerCtx, nil)
		if err == nil {
			t.Fatal("expected customer deletion to be forbidden, got nil")
		}

		// Partner owner can delete with input Reason
		reason := "Business closing"
		delRes, err := mResolver.DeleteDeliveryPartner(partnerCtx, &reason)
		if err != nil {
			t.Fatalf("expected partner deletion to succeed, got %v", err)
		}
		if !delRes.Success {
			t.Fatal("expected success true")
		}
		if delRes.Status != "deleted" {
			t.Fatalf("expected status 'deleted', got %s", delRes.Status)
		}
		if delRes.Message != "account deleted successfully" {
			t.Fatalf("expected message 'account deleted successfully', got %s", delRes.Message)
		}
	})

	t.Run("AppealDeliveryPartnerRejection validation", func(t *testing.T) {
		// Customer cannot appeal
		_, err := mResolver.AppealDeliveryPartnerRejection(customerCtx, "Reason")
		if err == nil {
			t.Fatal("expected customer appeal to be forbidden, got nil")
		}

		// Empty reason fails
		_, err = mResolver.AppealDeliveryPartnerRejection(partnerCtx, "   ")
		if err == nil {
			t.Fatal("expected empty reason to fail, got nil")
		}

		// Valid appeal succeeds
		appRes, err := mResolver.AppealDeliveryPartnerRejection(partnerCtx, "Valid Appeal Reason")
		if err != nil {
			t.Fatalf("expected appeal to succeed, got %v", err)
		}
		if appRes.ApprovalStatus != model.PartnerApprovalStatusAppealed {
			t.Fatalf("expected APPEALED status, got %s", appRes.ApprovalStatus)
		}
	})

	t.Run("ApproveDeliveryPartner admin flow", func(t *testing.T) {
		_, err := mResolver.ApproveDeliveryPartner(adminCtx, "  ")
		if err == nil {
			t.Fatal("expected empty partnerId to fail, got nil")
		}

		apprRes, err := mResolver.ApproveDeliveryPartner(adminCtx, "partner-uuid-1")
		if err != nil {
			t.Fatalf("expected approve to succeed, got %v", err)
		}
		if apprRes.ApprovalStatus != model.PartnerApprovalStatusApproved {
			t.Fatalf("expected APPROVED status, got %s", apprRes.ApprovalStatus)
		}
		if !apprRes.IsGstinVerified {
			t.Fatal("expected is_gstin_verified to be true")
		}
	})

	t.Run("RejectDeliveryPartner admin flow", func(t *testing.T) {
		_, err := mResolver.RejectDeliveryPartner(adminCtx, "partner-uuid-1", "  ")
		if err == nil {
			t.Fatal("expected empty reason to fail, got nil")
		}

		rejRes, err := mResolver.RejectDeliveryPartner(adminCtx, "partner-uuid-1", "Rejection Reason")
		if err != nil {
			t.Fatalf("expected reject to succeed, got %v", err)
		}
		if rejRes.ApprovalStatus != model.PartnerApprovalStatusRejected {
			t.Fatalf("expected REJECTED status, got %s", rejRes.ApprovalStatus)
		}
	})

	t.Run("AppealDeliveryPartners query", func(t *testing.T) {
		// Non-admin query fails
		_, err := qResolver.AppealDeliveryPartners(partnerCtx)
		if err == nil {
			t.Fatal("expected non-admin appealDeliveryPartners to be forbidden, got nil")
		}

		partners, err := qResolver.AppealDeliveryPartners(adminCtx)
		if err != nil {
			t.Fatalf("expected query to succeed, got %v", err)
		}
		if len(partners) != 1 {
			t.Fatalf("expected 1 appealed partner, got %d", len(partners))
		}
		if partners[0].ApprovalStatus != model.PartnerApprovalStatusAppealed {
			t.Fatalf("expected APPEALED partner, got %s", partners[0].ApprovalStatus)
		}
	})
}

func TestCreateDelivery_OrderLookupFails_Fails(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	orderMock := &mockOrderClientForDeliveryTest{
		err: appErrors.NotFound("order not found"),
	}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		OrderClient:    orderMock,
	}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-1", Role: "ADMIN"})
	_, err := r.CreateDelivery(ctx, model.CreateDeliveryInput{
		OrderID:           "ord-missing",
		DeliveryPartnerID: "partner-uuid-1",
		ShippingAddressID: "addr-1",
	})
	if err == nil {
		t.Fatal("expected error when order lookup fails, got nil")
	}
}

func TestCreateDelivery_NilOrIncompleteOrderResponse_Fails(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	// Incomplete order response (missing order id)
	orderMockIncomplete := &mockOrderClientForDeliveryTest{
		resp: &orderpb.GetOrderResponse{Order: &orderpb.Order{Id: "", PaymentStatus: "PAID"}},
	}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		OrderClient:    orderMockIncomplete,
	}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-1", Role: "ADMIN"})
	_, err := r.CreateDelivery(ctx, model.CreateDeliveryInput{
		OrderID:           "ord-incomplete",
		DeliveryPartnerID: "partner-uuid-1",
		ShippingAddressID: "addr-1",
	})
	if err == nil {
		t.Fatal("expected error for incomplete order response, got nil")
	}
	if !strings.Contains(err.Error(), "incomplete or invalid order response") {
		t.Errorf("expected 'incomplete or invalid order response' error, got %v", err)
	}
}

func TestCreateDelivery_OrderPaymentStatusEmpty_Fails(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	orderMock := &mockOrderClientForDeliveryTest{
		order: &orderpb.Order{
			Id:            "ord-unpaid",
			PaymentStatus: "",
		},
	}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		OrderClient:    orderMock,
	}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-1", Role: "ADMIN"})
	_, err := r.CreateDelivery(ctx, model.CreateDeliveryInput{
		OrderID:           "ord-unpaid",
		DeliveryPartnerID: "partner-uuid-1",
		ShippingAddressID: "addr-1",
	})
	if err == nil {
		t.Fatal("expected error when order payment_status is empty, got nil")
	}
	if !strings.Contains(err.Error(), "payment_status must be PAID") {
		t.Errorf("expected 'payment_status must be PAID' error, got %v", err)
	}
}


func TestCancelDelivery_Success(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	orderMock := &mockOrderClientForDeliveryTest{
		order: &orderpb.Order{Id: "ord-123", UserId: "usr-1"},
	}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		OrderClient:    orderMock,
	}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-1", Role: "CUSTOMER"})

	reason := "Changed mind"
	res, err := r.CancelDelivery(ctx, "del-123", &reason)
	if err != nil {
		t.Fatalf("CancelDelivery failed: %v", err)
	}
	if res.Status != model.DeliveryStatusCancelled {
		t.Errorf("expected CANCELLED status, got %v", res.Status)
	}
}

func TestCancelDelivery_ForbiddenForDifferentUser(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	orderMock := &mockOrderClientForDeliveryTest{
		order: &orderpb.Order{Id: "ord-123", UserId: "usr-buyer"},
	}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		OrderClient:    orderMock,
	}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}
	// Authenticated as a different user
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-intruder", Role: "CUSTOMER"})

	_, err := r.CancelDelivery(ctx, "del-123", nil)
	if err == nil {
		t.Fatal("expected forbidden error for different user, got nil")
	}
	if !strings.Contains(err.Error(), "only the user who placed the order can cancel this delivery") {
		t.Errorf("expected access denied error, got %v", err)
	}
}

func TestCancelDelivery_ForbiddenForAdmin(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	orderMock := &mockOrderClientForDeliveryTest{
		order: &orderpb.Order{Id: "ord-123", UserId: "usr-admin"},
	}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		OrderClient:    orderMock,
	}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-admin", Role: "ADMIN"})

	_, err := r.CancelDelivery(ctx, "del-123", nil)
	if err == nil {
		t.Fatal("expected forbidden error for admin, got nil")
	}
	if !strings.Contains(err.Error(), "admins do not have permission") {
		t.Errorf("expected admin forbidden error, got %v", err)
	}
}

func TestCancelDelivery_Unauthenticated(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	orderMock := &mockOrderClientForDeliveryTest{
		order: &orderpb.Order{Id: "ord-123", UserId: "usr-buyer"},
	}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		OrderClient:    orderMock,
	}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}
	ctx := context.Background()

	_, err := r.CancelDelivery(ctx, "del-123", nil)
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("expected authentication required error, got %v", err)
	}
}

func TestCancelDelivery_DeliveryNotFound(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	orderMock := &mockOrderClientForDeliveryTest{
		order: &orderpb.Order{Id: "ord-123", UserId: "usr-1"},
	}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		OrderClient:    orderMock,
	}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-1", Role: "CUSTOMER"})

	_, err := r.CancelDelivery(ctx, "not-found", nil)
	if err == nil {
		t.Fatal("expected not found error, got nil")
	}
	if !strings.Contains(err.Error(), "delivery not found") {
		t.Errorf("expected delivery not found error, got %v", err)
	}
}

func TestCancelDelivery_OrderNotFound(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	orderMock := &mockOrderClientForDeliveryTest{
		err: appErrors.NotFound("associated order not found"),
	}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		OrderClient:    orderMock,
	}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-1", Role: "CUSTOMER"})

	_, err := r.CancelDelivery(ctx, "del-123", nil)
	if err == nil {
		t.Fatal("expected order not found error, got nil")
	}
	if !strings.Contains(err.Error(), "associated order not found") {
		t.Errorf("expected order not found error, got %v", err)
	}
}

func TestCancelDelivery_FailedPrecondition(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	orderMock := &mockOrderClientForDeliveryTest{
		order: &orderpb.Order{Id: "ord-123", UserId: "usr-1"},
	}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		OrderClient:    orderMock,
	}
	r := &mutationResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-1", Role: "CUSTOMER"})

	_, err := r.CancelDelivery(ctx, "fail-id", nil)
	if err == nil {
		t.Fatal("expected error cancelling picked up delivery, got nil")
	}
	if !strings.Contains(err.Error(), "already picked up") {
		t.Errorf("expected 'already picked up' in error, got %v", err)
	}
}

func TestCalculateShippingFee_Success(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	clients := &grpc.Clients{DeliveryClient: deliveryMock}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-1", Role: "CUSTOMER"})

	weight := 450.0
	res, err := r.CalculateShippingFee(ctx, model.CalculateShippingFeeInput{
		DeliveryPartnerID: "partner-default",
		WeightGrams:       &weight,
	})
	if err != nil {
		t.Fatalf("CalculateShippingFee failed: %v", err)
	}
	if res.ShippingFee != 55.0 {
		t.Errorf("expected fee 55.0, got %v", res.ShippingFee)
	}
	if res.Zone != "TAMIL_NADU" {
		t.Errorf("expected zone TAMIL_NADU, got %v", res.Zone)
	}
}

func TestCalculateShippingFee_MissingDeliveryPartnerID_Fails(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	clients := &grpc.Clients{DeliveryClient: deliveryMock}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-1", Role: "CUSTOMER"})

	weight := 450.0
	_, err := r.CalculateShippingFee(ctx, model.CalculateShippingFeeInput{
		DeliveryPartnerID: "   ",
		WeightGrams:       &weight,
	})
	if err == nil {
		t.Fatal("expected error when deliveryPartnerId is empty, got nil")
	}
	if !strings.Contains(err.Error(), "deliveryPartnerId is required") {
		t.Errorf("expected 'deliveryPartnerId is required' error, got %v", err)
	}
}

func TestCalculateShippingFee_WithUserAddressAndDeliveryPartner(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	userMock := &mockUserClientForDeliveryTest{
		address: &userpb.Address{
			AddressLine: "123 Main St",
			City:        "Chennai",
			State:       "Tamil Nadu",
			PostalCode:  "600001",
			Country:     "India",
		},
	}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		UserClient:     userMock,
	}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-1", Role: "CUSTOMER"})

	addrID := "addr-chennai-1"
	partnerID := "partner-dhl"
	weight := 600.0
	res, err := r.CalculateShippingFee(ctx, model.CalculateShippingFeeInput{
		DeliveryPartnerID: partnerID,
		WeightGrams:       &weight,
		ShippingAddressID: &addrID,
	})
	if err != nil {
		t.Fatalf("CalculateShippingFee failed: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil response")
	}
	if deliveryMock.lastCalcReq == nil {
		t.Fatal("expected deliveryMock.lastCalcReq to be populated")
	}
	if deliveryMock.lastCalcReq.DestinationCity != "Chennai" {
		t.Errorf("expected destination city Chennai, got %s", deliveryMock.lastCalcReq.DestinationCity)
	}
	if deliveryMock.lastCalcReq.DestinationState != "Tamil Nadu" {
		t.Errorf("expected destination state Tamil Nadu, got %s", deliveryMock.lastCalcReq.DestinationState)
	}
	if deliveryMock.lastCalcReq.PartnerId != "partner-dhl" {
		t.Errorf("expected partner id partner-dhl, got %s", deliveryMock.lastCalcReq.PartnerId)
	}
}

func TestCalculateShippingFee_WithOrder(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	orderMock := &mockOrderClientForDeliveryTest{
		order: &orderpb.Order{
			Id:              "ord-fee-1",
			UserId:          "usr-1",
			ShippingAddress: "{\"city\":\"Coimbatore\",\"state\":\"Tamil Nadu\",\"postal_code\":\"641001\"}",
			Items: []*orderpb.OrderItem{
				{ProductId: "prod-1", Quantity: 2},
			},
		},
	}
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
		OrderClient:    orderMock,
	}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-1", Role: "CUSTOMER"})

	orderID := "ord-fee-1"
	partner := "express-courier"
	res, err := r.CalculateShippingFee(ctx, model.CalculateShippingFeeInput{
		DeliveryPartnerID: partner,
		OrderID:           &orderID,
	})
	if err != nil {
		t.Fatalf("CalculateShippingFee failed: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil response")
	}
	if deliveryMock.lastCalcReq == nil {
		t.Fatal("expected deliveryMock.lastCalcReq to be populated")
	}
	if deliveryMock.lastCalcReq.DestinationCity != "Coimbatore" {
		t.Errorf("expected destination city Coimbatore, got %s", deliveryMock.lastCalcReq.DestinationCity)
	}
	if deliveryMock.lastCalcReq.PartnerId != "express-courier" {
		t.Errorf("expected partner express-courier, got %s", deliveryMock.lastCalcReq.PartnerId)
	}
	if len(deliveryMock.lastCalcReq.Items) != 1 {
		t.Errorf("expected 1 item from order, got %d", len(deliveryMock.lastCalcReq.Items))
	}
}

func TestDeliveryByTracking_Success(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	clients := &grpc.Clients{DeliveryClient: deliveryMock}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-1", Role: "CUSTOMER"})

	res, err := r.DeliveryByTracking(ctx, "TRK-12345")
	if err != nil {
		t.Fatalf("DeliveryByTracking failed: %v", err)
	}
	if res == nil || res.ID != "del-123" {
		t.Errorf("expected del-123, got %v", res)
	}
}

func TestDeliveryByOrder_Success(t *testing.T) {
	deliveryMock := &mockDeliveryClient{}
	clients := &grpc.Clients{DeliveryClient: deliveryMock}
	r := &queryResolver{Resolver: &Resolver{Clients: clients}}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{UserID: "usr-1", Role: "CUSTOMER"})

	res, err := r.DeliveryByOrder(ctx, "ord-12345")
	if err != nil {
		t.Fatalf("DeliveryByOrder failed: %v", err)
	}
	if res == nil || res.ID != "del-123" {
		t.Errorf("expected del-123, got %v", res)
	}
}