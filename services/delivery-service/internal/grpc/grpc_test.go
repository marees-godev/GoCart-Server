package grpc_test

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/dto"
	deliveryGRPC "github.com/marees-godev/GoCart-Server/services/delivery-service/internal/grpc"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type mockDeliveryService struct {
	deliveries      map[string]*model.Delivery
	byOrder         map[string]string
	byTracking      map[string]string
	partnerProfiles map[string]*model.DeliveryPartnerProfile
}

func newMockDeliveryService() *mockDeliveryService {
	return &mockDeliveryService{
		deliveries:      make(map[string]*model.Delivery),
		byOrder:         make(map[string]string),
		byTracking:      make(map[string]string),
		partnerProfiles: make(map[string]*model.DeliveryPartnerProfile),
	}
}

func (m *mockDeliveryService) HandleOrderConfirmed(ctx context.Context, payload *dto.OrderConfirmedPayload) (*model.Delivery, error) {
	if payload == nil || payload.OrderID == "" {
		return nil, appErrors.BadRequest("order_id is required")
	}
	if id, exists := m.byOrder[payload.OrderID]; exists {
		return m.deliveries[id], nil
	}

	d := &model.Delivery{
		ID:              "del-123",
		OrderID:         payload.OrderID,
		DeliveryPartner: payload.DeliveryPartner,
		TrackingNumber:  "TRK-12345",
		Status:          model.StatusCreated,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if payload.ShippingAddress != nil {
		d.ShippingAddress = *payload.ShippingAddress
	}

	m.deliveries[d.ID] = d
	m.byOrder[d.OrderID] = d.ID
	m.byTracking[d.TrackingNumber] = d.ID
	return d, nil
}

func (m *mockDeliveryService) GetDeliveryByOrderID(ctx context.Context, orderID string) (*model.Delivery, error) {
	id, exists := m.byOrder[orderID]
	if !exists {
		return nil, appErrors.NotFound("delivery not found for order")
	}
	return m.deliveries[id], nil
}

func (m *mockDeliveryService) GetDeliveryByTrackingNumber(ctx context.Context, trackingNumber string) (*model.Delivery, error) {
	id, exists := m.byTracking[trackingNumber]
	if !exists {
		return nil, appErrors.NotFound("delivery not found for tracking number")
	}
	return m.deliveries[id], nil
}

func (m *mockDeliveryService) GetDeliveryByID(ctx context.Context, id string) (*model.Delivery, error) {
	d, exists := m.deliveries[id]
	if !exists {
		return nil, appErrors.NotFound("delivery not found")
	}
	return d, nil
}

func (m *mockDeliveryService) ListDeliveries(ctx context.Context, limit, offset int, status string) ([]*model.Delivery, int, error) {
	var list []*model.Delivery
	for _, d := range m.deliveries {
		if status == "" || string(d.Status) == status {
			list = append(list, d)
		}
	}
	return list, len(list), nil
}

func (m *mockDeliveryService) UpdateStatus(ctx context.Context, id string, targetStatus model.DeliveryStatus) (*model.Delivery, error) {
	return m.ProcessStatusUpdate(ctx, &dto.DeliveryStatusUpdateRequest{
		DeliveryID: id,
		Status:     targetStatus,
	})
}

func (m *mockDeliveryService) ProcessStatusUpdate(ctx context.Context, req *dto.DeliveryStatusUpdateRequest) (*model.Delivery, error) {
	if req == nil {
		return nil, appErrors.BadRequest("request is required")
	}
	cleanStatus := model.DeliveryStatus(strings.ToUpper(strings.TrimSpace(string(req.Status))))
	if !model.IsValidStatus(cleanStatus) {
		return nil, appErrors.BadRequest("invalid status")
	}

	id := strings.TrimSpace(req.DeliveryID)
	if id == "" {
		id = strings.TrimSpace(req.TrackingNumber)
	}
	if id == "" {
		id = strings.TrimSpace(req.OrderID)
	}

	d, exists := m.deliveries[id]
	if !exists {
		if delID, ok := m.byOrder[id]; ok {
			d = m.deliveries[delID]
			exists = true
		} else if delID, ok := m.byTracking[id]; ok {
			d = m.deliveries[delID]
			exists = true
		}
	}
	if !exists {
		return nil, appErrors.NotFound("delivery not found")
	}

	if d.Status == cleanStatus {
		if req.EstimatedDelivery != nil {
			d.EstimatedDelivery = req.EstimatedDelivery
		}
		if req.ActualDelivery != nil {
			d.ActualDelivery = req.ActualDelivery
		}
		return d, nil
	}

	if err := d.TransitionStatus(cleanStatus); err != nil {
		return nil, err
	}

	if req.EstimatedDelivery != nil {
		d.EstimatedDelivery = req.EstimatedDelivery
	}
	if cleanStatus == model.StatusDelivered {
		if req.ActualDelivery != nil {
			d.ActualDelivery = req.ActualDelivery
		} else if d.ActualDelivery == nil {
			now := time.Now().UTC()
			d.ActualDelivery = &now
		}
	} else if req.ActualDelivery != nil {
		d.ActualDelivery = req.ActualDelivery
	}

	d.Status = cleanStatus
	d.UpdatedAt = time.Now().UTC()
	return d, nil
}

func (m *mockDeliveryService) EstimateDelivery(ctx context.Context, orderID string, partnerID ...string) (*dto.EstimatedDeliveryResponse, error) {
	return nil, nil
}



func (m *mockDeliveryService) CreateOrUpdatePartnerProfile(ctx context.Context, profile *model.DeliveryPartnerProfile) error {
	if m.partnerProfiles == nil {
		m.partnerProfiles = make(map[string]*model.DeliveryPartnerProfile)
	}
	m.partnerProfiles[profile.PartnerID.String()] = profile
	return nil
}

func (m *mockDeliveryService) GetPartnerProfile(ctx context.Context, partnerID string) (*model.DeliveryPartnerProfile, error) {
	if m.partnerProfiles == nil {
		return nil, appErrors.NotFound("partner profile not found")
	}
	p, ok := m.partnerProfiles[partnerID]
	if !ok {
		return nil, appErrors.NotFound("partner profile not found")
	}
	return p, nil
}

func (m *mockDeliveryService) UpdatePartnerProfile(ctx context.Context, partnerID string, companyName, email, contactNumber *string) (*model.DeliveryPartnerProfile, error) {
	if m.partnerProfiles == nil {
		m.partnerProfiles = make(map[string]*model.DeliveryPartnerProfile)
	}
	p, ok := m.partnerProfiles[partnerID]
	if !ok {
		pUUID, err := uuid.Parse(partnerID)
		if err != nil {
			return nil, appErrors.BadRequest("invalid partner_id format")
		}
		p = &model.DeliveryPartnerProfile{
			ID:        uuid.New(),
			PartnerID: pUUID,
			IsActive:  true,
			CreatedAt: time.Now().UTC(),
		}
	}
	if companyName != nil {
		p.CompanyName = *companyName
	}
	if email != nil {
		p.Email = *email
	}
	if contactNumber != nil {
		p.ContactNumber = *contactNumber
	}
	p.UpdatedAt = time.Now().UTC()
	m.partnerProfiles[partnerID] = p
	return p, nil
}


func setupTestGRPCServer(t *testing.T, svc *mockDeliveryService) (deliverypb.DeliveryServiceClient, func()) {
	lis := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	deliveryGRPCServer := deliveryGRPC.NewDeliveryGRPCServer(svc)
	deliverypb.RegisterDeliveryServiceServer(server, deliveryGRPCServer)

	go func() {
		_ = server.Serve(lis)
	}()

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial bufnet: %v", err)
	}

	client := deliverypb.NewDeliveryServiceClient(conn)
	cleanup := func() {
		_ = conn.Close()
		server.Stop()
		_ = lis.Close()
	}

	return client, cleanup
}

func TestGRPC_CreateDelivery(t *testing.T) {
	svc := newMockDeliveryService()
	client, cleanup := setupTestGRPCServer(t, svc)
	defer cleanup()

	resp, err := client.CreateDelivery(context.Background(), &deliverypb.CreateDeliveryRequest{
		OrderId:         "order-101",
		DeliveryPartner: "BLUEDART",
		ShippingAddress: `{"street":"123 Main St","city":"Mumbai"}`,
		ContactNumber:   "+919876543210",
	})
	if err != nil {
		t.Fatalf("CreateDelivery failed: %v", err)
	}
	if resp.Delivery == nil {
		t.Fatal("expected delivery in response, got nil")
	}
	if resp.Delivery.OrderId != "order-101" {
		t.Errorf("expected order_id 'order-101', got %s", resp.Delivery.OrderId)
	}
	if resp.Delivery.DeliveryPartner != "BLUEDART" {
		t.Errorf("expected partner 'BLUEDART', got %s", resp.Delivery.DeliveryPartner)
	}
	if resp.Delivery.ContactNumber != "+919876543210" {
		t.Errorf("expected contact_number '+919876543210', got %s", resp.Delivery.ContactNumber)
	}
}

func TestGRPC_GetDeliveryByID(t *testing.T) {
	svc := newMockDeliveryService()
	svc.deliveries["del-999"] = &model.Delivery{
		ID:              "del-999",
		OrderID:         "order-999",
		TrackingNumber:  "TRK-999",
		DeliveryPartner: "DELHIVERY",
		Status:          model.StatusCreated,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	client, cleanup := setupTestGRPCServer(t, svc)
	defer cleanup()

	resp, err := client.GetDelivery(context.Background(), &deliverypb.GetDeliveryRequest{Id: "del-999"})
	if err != nil {
		t.Fatalf("GetDelivery failed: %v", err)
	}
	if resp.Delivery.Id != "del-999" {
		t.Errorf("expected id 'del-999', got %s", resp.Delivery.Id)
	}

	// Not found
	_, err = client.GetDelivery(context.Background(), &deliverypb.GetDeliveryRequest{Id: "nonexistent"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound code, got %v", status.Code(err))
	}
}

func TestGRPC_UpdateDeliveryStatus(t *testing.T) {
	svc := newMockDeliveryService()
	svc.deliveries["del-status-test"] = &model.Delivery{
		ID:        "del-status-test",
		OrderID:   "order-status-test",
		Status:    model.StatusCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	client, cleanup := setupTestGRPCServer(t, svc)
	defer cleanup()

	// Valid transition: CREATED -> PICKED_UP
	resp, err := client.UpdateDeliveryStatus(context.Background(), &deliverypb.UpdateDeliveryStatusRequest{
		Id:     "del-status-test",
		Status: "PICKED_UP",
	})
	if err != nil {
		t.Fatalf("UpdateDeliveryStatus failed: %v", err)
	}
	if resp.Delivery.Status != "PICKED_UP" {
		t.Errorf("expected status PICKED_UP, got %s", resp.Delivery.Status)
	}

	// Idempotent transition: PICKED_UP -> PICKED_UP
	respSame, err := client.UpdateDeliveryStatus(context.Background(), &deliverypb.UpdateDeliveryStatusRequest{
		Id:     "del-status-test",
		Status: "PICKED_UP",
	})
	if err != nil {
		t.Fatalf("idempotent UpdateDeliveryStatus failed: %v", err)
	}
	if respSame.Delivery.Status != "PICKED_UP" {
		t.Errorf("expected status PICKED_UP, got %s", respSame.Delivery.Status)
	}

	// Invalid transition: PICKED_UP -> DELIVERED (skipped IN_TRANSIT, OUT_FOR_DELIVERY)
	_, err = client.UpdateDeliveryStatus(context.Background(), &deliverypb.UpdateDeliveryStatusRequest{
		Id:     "del-status-test",
		Status: "DELIVERED",
	})
	if err == nil {
		t.Fatal("expected error on invalid transition, got nil")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument code on invalid transition, got %v", status.Code(err))
	}
}

func TestGRPC_PartnerProfile(t *testing.T) {
	svc := newMockDeliveryService()
	client, cleanup := setupTestGRPCServer(t, svc)
	defer cleanup()

	ctx := context.Background()
	partnerID := uuid.New().String()

	// 1. CreatePartnerProfile
	createResp, err := client.CreatePartnerProfile(ctx, &deliverypb.CreatePartnerProfileRequest{
		PartnerId:     partnerID,
		CompanyName:   "Flash Courier",
		Email:         "flash@example.com",
		ContactNumber: "+1-555-0199",
	})
	if err != nil {
		t.Fatalf("CreatePartnerProfile failed: %v", err)
	}
	if createResp.Profile.PartnerId != partnerID {
		t.Errorf("expected partner_id %s, got %s", partnerID, createResp.Profile.PartnerId)
	}
	if createResp.Profile.CompanyName != "Flash Courier" {
		t.Errorf("expected company_name 'Flash Courier', got %s", createResp.Profile.CompanyName)
	}

	// 1b. CreatePartnerProfile with empty company_name (registration flow)
	emptyPartnerID := uuid.New().String()
	emptyCompanyResp, err := client.CreatePartnerProfile(ctx, &deliverypb.CreatePartnerProfileRequest{
		PartnerId:     emptyPartnerID,
		CompanyName:   "",
		Email:         "partner.register@example.com",
		ContactNumber: "",
	})
	if err != nil {
		t.Fatalf("CreatePartnerProfile with empty company_name failed: %v", err)
	}
	if emptyCompanyResp.Profile.PartnerId != emptyPartnerID {
		t.Errorf("expected partner_id %s, got %s", emptyPartnerID, emptyCompanyResp.Profile.PartnerId)
	}

	// 2. GetPartnerProfile
	getResp, err := client.GetPartnerProfile(ctx, &deliverypb.GetPartnerProfileRequest{
		PartnerId: partnerID,
	})
	if err != nil {
		t.Fatalf("GetPartnerProfile failed: %v", err)
	}
	if getResp.Profile.Email != "flash@example.com" {
		t.Errorf("expected email 'flash@example.com', got %s", getResp.Profile.Email)
	}

	// 3. UpdatePartnerProfile
	newCompany := "Flash Logistics Worldwide"
	newContact := "+1-555-9999"
	updateResp, err := client.UpdatePartnerProfile(ctx, &deliverypb.UpdatePartnerProfileRequest{
		PartnerId:     partnerID,
		CompanyName:   &newCompany,
		ContactNumber: &newContact,
	})
	if err != nil {
		t.Fatalf("UpdatePartnerProfile failed: %v", err)
	}
	if updateResp.Profile.CompanyName != "Flash Logistics Worldwide" {
		t.Errorf("expected updated company_name, got %s", updateResp.Profile.CompanyName)
	}
	if updateResp.Profile.ContactNumber != "+1-555-9999" {
		t.Errorf("expected updated contact_number, got %s", updateResp.Profile.ContactNumber)
	}
}




