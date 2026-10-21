package tests

import (
	"context"
	"testing"

	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	grpcPkg "google.golang.org/grpc"
)

type mockDeliveryClientForResolvers struct {
	deliverypb.DeliveryServiceClient
	lastGetProfileReq    *deliverypb.GetPartnerProfileRequest
	lastUpdateProfileReq *deliverypb.UpdatePartnerProfileRequest
	profiles             map[string]*deliverypb.DeliveryPartnerProfile
}

func (m *mockDeliveryClientForResolvers) CreatePartnerProfile(ctx context.Context, in *deliverypb.CreatePartnerProfileRequest, opts ...grpcPkg.CallOption) (*deliverypb.PartnerProfileResponse, error) {
	p := &deliverypb.DeliveryPartnerProfile{
		Id:            "prof-new-1",
		PartnerId:     in.PartnerId,
		CompanyName:   in.CompanyName,
		Email:         in.Email,
		ContactNumber: in.ContactNumber,
		IsActive:      true,
		CreatedAt:     "2026-10-21T12:00:00Z",
		UpdatedAt:     "2026-10-21T12:00:00Z",
	}
	m.profiles[in.PartnerId] = p
	return &deliverypb.PartnerProfileResponse{Profile: p}, nil
}

func newMockDeliveryClientForResolvers() *mockDeliveryClientForResolvers {
	return &mockDeliveryClientForResolvers{
		profiles: make(map[string]*deliverypb.DeliveryPartnerProfile),
	}
}

func (m *mockDeliveryClientForResolvers) GetPartnerProfile(ctx context.Context, in *deliverypb.GetPartnerProfileRequest, opts ...grpcPkg.CallOption) (*deliverypb.PartnerProfileResponse, error) {
	m.lastGetProfileReq = in
	p, ok := m.profiles[in.PartnerId]
	if !ok {
		p = &deliverypb.DeliveryPartnerProfile{
			Id:            "prof-1",
			PartnerId:     in.PartnerId,
			CompanyName:   "Express Delivery LLC",
			Email:         "partner@example.com",
			ContactNumber: "+1-206-555-0100",
			IsActive:      true,
			CreatedAt:     "2026-10-21T00:00:00Z",
			UpdatedAt:     "2026-10-21T00:00:00Z",
		}
	}
	return &deliverypb.PartnerProfileResponse{Profile: p}, nil
}

func (m *mockDeliveryClientForResolvers) UpdatePartnerProfile(ctx context.Context, in *deliverypb.UpdatePartnerProfileRequest, opts ...grpcPkg.CallOption) (*deliverypb.PartnerProfileResponse, error) {
	m.lastUpdateProfileReq = in
	p, ok := m.profiles[in.PartnerId]
	if !ok {
		p = &deliverypb.DeliveryPartnerProfile{
			Id:        "prof-1",
			PartnerId: in.PartnerId,
			IsActive:  true,
		}
	}
	if in.CompanyName != "" {
		p.CompanyName = in.CompanyName
	}
	if in.Email != "" {
		p.Email = in.Email
	}
	if in.ContactNumber != "" {
		p.ContactNumber = in.ContactNumber
	}
	p.UpdatedAt = "2026-10-21T12:00:00Z"
	m.profiles[in.PartnerId] = p
	return &deliverypb.PartnerProfileResponse{Profile: p}, nil
}

func TestDeliveryResolver_GetPartnerProfile(t *testing.T) {
	deliveryMock := newMockDeliveryClientForResolvers()
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
	}

	resolver := &resolvers.Resolver{
		Clients: clients,
	}

	partnerID := "partner-uuid-1234"
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:    partnerID,
		Role:      "DELIVERY_PARTNER",
		PartnerID: partnerID,
		Email:     "partner@example.com",
	})

	// 1. As DELIVERY_PARTNER querying their own profile
	res, err := resolver.Query().DeliveryPartnerProfile(ctx, nil)
	if err != nil {
		t.Fatalf("DeliveryPartnerProfile failed: %v", err)
	}
	if res.PartnerID != partnerID {
		t.Errorf("expected partnerId %s, got %s", partnerID, res.PartnerID)
	}
	if deliveryMock.lastGetProfileReq.PartnerId != partnerID {
		t.Errorf("expected requested partnerId %s, got %s", partnerID, deliveryMock.lastGetProfileReq.PartnerId)
	}

	// 2. As ADMIN querying specific partner
	adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	})
	otherPartner := "other-partner-999"
	adminRes, err := resolver.Query().DeliveryPartnerProfile(adminCtx, &otherPartner)
	if err != nil {
		t.Fatalf("Admin query DeliveryPartnerProfile failed: %v", err)
	}
	if adminRes.PartnerID != otherPartner {
		t.Errorf("expected partnerId %s, got %s", otherPartner, adminRes.PartnerID)
	}
	if deliveryMock.lastGetProfileReq.PartnerId != otherPartner {
		t.Errorf("expected requested partnerId %s, got %s", otherPartner, deliveryMock.lastGetProfileReq.PartnerId)
	}
}

func TestDeliveryResolver_UpdatePartnerProfile(t *testing.T) {
	deliveryMock := newMockDeliveryClientForResolvers()
	clients := &grpc.Clients{
		DeliveryClient: deliveryMock,
	}

	resolver := &resolvers.Resolver{
		Clients: clients,
	}

	partnerID := "partner-uuid-5678"
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:    partnerID,
		Role:      "DELIVERY_PARTNER",
		PartnerID: partnerID,
		Email:     "partner@example.com",
	})

	newCompany := "Speedy Deliveries Ltd"
	newContact := "+1-555-4321"
	res, err := resolver.Mutation().UpdateDeliveryPartnerProfile(ctx, model.UpdateDeliveryPartnerProfileInput{
		CompanyName:   &newCompany,
		ContactNumber: &newContact,
	})
	if err != nil {
		t.Fatalf("UpdateDeliveryPartnerProfile failed: %v", err)
	}
	if res.CompanyName == nil || *res.CompanyName != newCompany {
		t.Errorf("expected companyName %s, got %v", newCompany, res.CompanyName)
	}
	if res.ContactNumber == nil || *res.ContactNumber != newContact {
		t.Errorf("expected contactNumber %s, got %v", newContact, res.ContactNumber)
	}
	if deliveryMock.lastUpdateProfileReq.PartnerId != partnerID {
		t.Errorf("expected partnerId %s in gRPC request, got %s", partnerID, deliveryMock.lastUpdateProfileReq.PartnerId)
	}
	if deliveryMock.lastUpdateProfileReq.CompanyName != newCompany {
		t.Errorf("expected companyName %s in gRPC request, got %s", newCompany, deliveryMock.lastUpdateProfileReq.CompanyName)
	}

	// Test unauthenticated error
	_, err = resolver.Mutation().UpdateDeliveryPartnerProfile(context.Background(), model.UpdateDeliveryPartnerProfileInput{
		CompanyName: &newCompany,
	})
	if err == nil {
		t.Fatal("expected error for unauthenticated user, got nil")
	}
}

