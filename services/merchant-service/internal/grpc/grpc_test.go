package grpc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/dto"
	merchantGRPC "github.com/marees-godev/GoCart-Server/services/merchant-service/internal/grpc"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type mockService struct {
	merchants map[uuid.UUID]*model.Merchant
}

func newMockService() *mockService {
	return &mockService{
		merchants: make(map[uuid.UUID]*model.Merchant),
	}
}

func (m *mockService) GetMerchantByID(ctx context.Context, id uuid.UUID) (*model.Merchant, error) {
	merch, exists := m.merchants[id]
	if !exists {
		return nil, appErrors.NotFound("merchant not found")
	}
	return merch, nil
}

func (m *mockService) CreateMerchant(ctx context.Context, req dto.CreateMerchantRequest) (*model.Merchant, error) {
	id, err := uuid.Parse(req.ID)
	if err != nil {
		id = uuid.New()
	}
	merch := &model.Merchant{
		ID:            id,
		BusinessName:  "",
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		BusinessEmail: req.BusinessEmail,
		BusinessPhone: "",
		TaxID:         "",
		Status:        string(model.MerchantStatusPending),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	m.merchants[id] = merch
	return merch, nil
}

func (m *mockService) ListMerchants(ctx context.Context, limit, offset int, status string) ([]*model.Merchant, int, error) {
	list := make([]*model.Merchant, 0)
	for _, merch := range m.merchants {
		if status == "" || merch.Status == status {
			list = append(list, merch)
		}
	}
	return list, len(list), nil
}

func (m *mockService) UpdateMerchant(ctx context.Context, id uuid.UUID, req dto.UpdateMerchantRequest) (*model.Merchant, error) {
	merch, exists := m.merchants[id]
	if !exists {
		return nil, appErrors.NotFound("merchant not found")
	}
	if req.BusinessName != "" {
		if len(req.BusinessName) < 2 || len(req.BusinessName) > 100 {
			return nil, appErrors.BadRequest("business_name must be between 2 and 100 characters")
		}
		merch.BusinessName = req.BusinessName
	}
	if req.BusinessPhone != "" {
		merch.BusinessPhone = req.BusinessPhone
	}
	if req.TaxID != "" {
		merch.TaxID = req.TaxID
	}
	merch.UpdatedAt = time.Now()
	return merch, nil
}

func (m *mockService) UpdateMerchantStatus(ctx context.Context, id uuid.UUID, req dto.UpdateMerchantStatusRequest) (*model.Merchant, error) {
	merch, exists := m.merchants[id]
	if !exists {
		return nil, appErrors.NotFound("merchant not found")
	}
	merch.Status = req.Status
	merch.RejectionReason = req.RejectionReason
	return merch, nil
}

func (m *mockService) DeleteMerchant(ctx context.Context, id uuid.UUID) error {
	if _, exists := m.merchants[id]; !exists {
		return appErrors.NotFound("merchant not found")
	}
	delete(m.merchants, id)
	return nil
}

func TestMerchantGRPC_CRUD(t *testing.T) {
	svc := newMockService()
	server := merchantGRPC.NewMerchantGRPCServer(svc)

	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	merchantpb.RegisterMerchantServiceServer(grpcServer, server)

	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial bufnet: %v", err)
	}
	defer conn.Close()

	client := merchantpb.NewMerchantServiceClient(conn)
	ctx := context.Background()

	// 1. CreateMerchant
	merchantUUID := uuid.New().String()
	createRes, err := client.CreateMerchant(ctx, &merchantpb.CreateMerchantRequest{
		Id:            merchantUUID,
		FirstName:     "John",
		LastName:      "Doe",
		BusinessEmail: "john@example.com",
	})
	if err != nil {
		t.Fatalf("CreateMerchant failed: %v", err)
	}
	if createRes.Merchant.Status != "PENDING" {
		t.Errorf("expected status PENDING, got %s", createRes.Merchant.Status)
	}
	if createRes.Merchant.BusinessName != "" {
		t.Errorf("expected initial business name to be empty, got %s", createRes.Merchant.BusinessName)
	}
	if createRes.Merchant.FirstName != "John" || createRes.Merchant.LastName != "Doe" {
		t.Errorf("expected name John Doe, got %s %s", createRes.Merchant.FirstName, createRes.Merchant.LastName)
	}
	if createRes.Merchant.BusinessEmail != "john@example.com" {
		t.Errorf("expected email john@example.com, got %s", createRes.Merchant.BusinessEmail)
	}
	if createRes.Merchant.CreatedAt == nil || createRes.Merchant.UpdatedAt == nil {
		t.Errorf("expected timestamp fields to be non-nil")
	}

	// Test non-merchant role forbidden
	custCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-user-role", "CUSTOMER"))
	_, err = client.CreateMerchant(custCtx, &merchantpb.CreateMerchantRequest{
		Id:            uuid.New().String(),
		FirstName:     "Customer",
		LastName:      "Merchant",
		BusinessEmail: "cust@example.com",
	})
	if err == nil {
		t.Errorf("expected error for non-merchant role, got nil")
	}

	merchantID := createRes.Merchant.Id

	// 2. GetMerchant
	getRes, err := client.GetMerchant(ctx, &merchantpb.GetMerchantRequest{Id: merchantID})
	if err != nil {
		t.Fatalf("GetMerchant failed: %v", err)
	}
	if getRes.Merchant.Id != merchantID {
		t.Errorf("expected id %s, got %s", merchantID, getRes.Merchant.Id)
	}
	if getRes.Merchant.BusinessName != "" {
		t.Errorf("expected empty business name prior to update, got %s", getRes.Merchant.BusinessName)
	}

	// 3. GetMerchant with invalid UUID returns InvalidArgument
	_, err = client.GetMerchant(ctx, &merchantpb.GetMerchantRequest{Id: "invalid-uuid"})
	if err == nil || status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for invalid merchant UUID, got %v", err)
	}

	// 4. GetMerchant with non-existent UUID returns NotFound
	_, err = client.GetMerchant(ctx, &merchantpb.GetMerchantRequest{Id: uuid.New().String()})
	if err == nil || status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound for non-existent merchant UUID, got %v", err)
	}

	// 5. ListMerchants
	listRes, err := client.ListMerchants(ctx, &merchantpb.ListMerchantsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListMerchants failed: %v", err)
	}
	if len(listRes.Merchants) != 1 {
		t.Errorf("expected 1 merchant, got %d", len(listRes.Merchants))
	}

	// 6. UpdateMerchant
	updateRes, err := client.UpdateMerchant(ctx, &merchantpb.UpdateMerchantRequest{
		Id:            merchantID,
		BusinessName:  "Best Merchant Updated",
		BusinessPhone: "+19876543210",
		TaxId:         "TAX-999",
	})
	if err != nil {
		t.Fatalf("UpdateMerchant failed: %v", err)
	}
	if updateRes.Merchant.BusinessName != "Best Merchant Updated" {
		t.Errorf("expected Best Merchant Updated, got %s", updateRes.Merchant.BusinessName)
	}
	if updateRes.Merchant.BusinessPhone != "+19876543210" {
		t.Errorf("expected +19876543210, got %s", updateRes.Merchant.BusinessPhone)
	}
	if updateRes.Merchant.TaxId != "TAX-999" {
		t.Errorf("expected TAX-999, got %s", updateRes.Merchant.TaxId)
	}
	// Verify non-editable fields remain unchanged
	if updateRes.Merchant.FirstName != "John" || updateRes.Merchant.LastName != "Doe" {
		t.Errorf("expected non-editable names to remain unchanged, got %s %s", updateRes.Merchant.FirstName, updateRes.Merchant.LastName)
	}
	if updateRes.Merchant.BusinessEmail != "john@example.com" {
		t.Errorf("expected non-editable email to remain unchanged, got %s", updateRes.Merchant.BusinessEmail)
	}
	if updateRes.Merchant.Status != "PENDING" {
		t.Errorf("expected status to remain PENDING, got %s", updateRes.Merchant.Status)
	}

	// 7. UpdateMerchant with non-existent UUID returns NotFound
	_, err = client.UpdateMerchant(ctx, &merchantpb.UpdateMerchantRequest{
		Id:            uuid.New().String(),
		BusinessName:  "NonExistent",
		BusinessPhone: "+1234567890",
		TaxId:         "TAX-001",
	})
	if err == nil || status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound for non-existent merchant on update, got %v", err)
	}

	// 8. UpdateMerchantStatus
	statusRes, err := client.UpdateMerchantStatus(ctx, &merchantpb.UpdateMerchantStatusRequest{
		Id:     merchantID,
		Status: "APPROVED",
	})
	if err != nil {
		t.Fatalf("UpdateMerchantStatus failed: %v", err)
	}
	if statusRes.Merchant.Status != "APPROVED" {
		t.Errorf("expected APPROVED, got %s", statusRes.Merchant.Status)
	}

	// 9. DeleteMerchant
	delRes, err := client.DeleteMerchant(ctx, &merchantpb.DeleteMerchantRequest{Id: merchantID})
	if err != nil {
		t.Fatalf("DeleteMerchant failed: %v", err)
	}
	if !delRes.Success {
		t.Errorf("expected success true, got false")
	}
}
