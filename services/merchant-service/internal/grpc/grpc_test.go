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
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
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

func (m *mockService) GetMerchantByUserID(ctx context.Context, userID uuid.UUID) (*model.Merchant, error) {
	for _, merch := range m.merchants {
		if merch.UserID == userID {
			return merch, nil
		}
	}
	return nil, appErrors.NotFound("merchant not found")
}

func (m *mockService) CreateMerchant(ctx context.Context, req dto.CreateMerchantRequest) (*model.Merchant, error) {
	id, err := uuid.Parse(req.ID)
	if err != nil {
		id = uuid.New()
	}
	userUUID, _ := uuid.Parse(req.UserID)
	if userUUID == uuid.Nil {
		userUUID = id
	}
	merch := &model.Merchant{
		ID:            id,
		UserID:        userUUID,
		BusinessName:  req.BusinessName,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		BusinessEmail: req.BusinessEmail,
		BusinessPhone: req.BusinessPhone,
		TaxID:         req.TaxID,
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
		merch.BusinessName = req.BusinessName
	}
	if req.FirstName != "" {
		merch.FirstName = req.FirstName
	}
	if req.LastName != "" {
		merch.LastName = req.LastName
	}
	if req.BusinessEmail != "" {
		merch.BusinessEmail = req.BusinessEmail
	}
	if req.BusinessPhone != "" {
		merch.BusinessPhone = req.BusinessPhone
	}
	if req.TaxID != "" {
		merch.TaxID = req.TaxID
	}
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
	userID := uuid.New().String()
	createRes, err := client.CreateMerchant(ctx, &merchantpb.CreateMerchantRequest{
		UserId:        userID,
		BusinessName:  "Best Merchant",
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
	if createRes.Merchant.FirstName != "John" || createRes.Merchant.LastName != "Doe" {
		t.Errorf("expected name John Doe, got %s %s", createRes.Merchant.FirstName, createRes.Merchant.LastName)
	}
	if createRes.Merchant.BusinessEmail != "john@example.com" {
		t.Errorf("expected email john@example.com, got %s", createRes.Merchant.BusinessEmail)
	}

	// Test non-merchant role forbidden
	custCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-user-role", "CUSTOMER"))
	_, err = client.CreateMerchant(custCtx, &merchantpb.CreateMerchantRequest{
		UserId:       uuid.New().String(),
		BusinessName: "Customer Merchant",
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

	// 3. GetMerchantByUserID
	byUserRes, err := client.GetMerchantByUserID(ctx, &merchantpb.GetMerchantByUserIDRequest{UserId: userID})
	if err != nil {
		t.Fatalf("GetMerchantByUserID failed: %v", err)
	}
	if byUserRes.Merchant.Id != merchantID {
		t.Errorf("expected id %s, got %s", merchantID, byUserRes.Merchant.Id)
	}

	// 4. ListMerchants
	listRes, err := client.ListMerchants(ctx, &merchantpb.ListMerchantsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListMerchants failed: %v", err)
	}
	if len(listRes.Merchants) != 1 {
		t.Errorf("expected 1 merchant, got %d", len(listRes.Merchants))
	}

	// 5. UpdateMerchant
	updateRes, err := client.UpdateMerchant(ctx, &merchantpb.UpdateMerchantRequest{
		Id:            merchantID,
		BusinessName:  "Best Merchant Updated",
		FirstName:     "Johnny",
		LastName:      "Doh",
		BusinessPhone: "+1234567890",
		TaxId:         "TAX-999",
	})
	if err != nil {
		t.Fatalf("UpdateMerchant failed: %v", err)
	}
	if updateRes.Merchant.BusinessName != "Best Merchant Updated" {
		t.Errorf("expected Best Merchant Updated, got %s", updateRes.Merchant.BusinessName)
	}
	if updateRes.Merchant.FirstName != "Johnny" || updateRes.Merchant.LastName != "Doh" {
		t.Errorf("expected Johnny Doh, got %s %s", updateRes.Merchant.FirstName, updateRes.Merchant.LastName)
	}
	if updateRes.Merchant.TaxId != "TAX-999" {
		t.Errorf("expected TAX-999, got %s", updateRes.Merchant.TaxId)
	}

	// 6. UpdateMerchantStatus
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

	// 7. DeleteMerchant
	delRes, err := client.DeleteMerchant(ctx, &merchantpb.DeleteMerchantRequest{Id: merchantID})
	if err != nil {
		t.Fatalf("DeleteMerchant failed: %v", err)
	}
	if !delRes.Success {
		t.Errorf("expected success true, got false")
	}
}
