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
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		BusinessEmail: req.BusinessEmail,
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

func (m *mockService) ListReactivatedMerchants(ctx context.Context, limit, offset int) ([]*model.Merchant, int, error) {
	list := make([]*model.Merchant, 0)
	for _, merch := range m.merchants {
		list = append(list, merch)
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
	if req.PanCardNumber != "" {
		merch.PanCardNumber = req.PanCardNumber
	}
	if req.FirstName != "" {
		merch.FirstName = req.FirstName
	}
	if req.LastName != "" {
		merch.LastName = req.LastName
	}
	merch.UpdatedAt = time.Now()
	return merch, nil
}

func (m *mockService) UpdateMerchantStatus(ctx context.Context, id uuid.UUID, req dto.UpdateMerchantStatusRequest) (*model.Merchant, string, error) {
	merch, exists := m.merchants[id]
	if !exists {
		return nil, "", appErrors.NotFound("merchant not found")
	}
	prev := merch.Status
	merch.Status = req.Status
	merch.RejectionReason = req.RejectionReason
	return merch, prev, nil
}

func (m *mockService) DeleteMerchant(ctx context.Context, id uuid.UUID) error {
	if _, exists := m.merchants[id]; !exists {
		return appErrors.NotFound("merchant not found")
	}
	delete(m.merchants, id)
	return nil
}

func (m *mockService) ActivateMerchant(ctx context.Context, id uuid.UUID, reason string, adminID string, reqID string) (*model.Merchant, model.MerchantStatus, error) {
	merch, exists := m.merchants[id]
	if !exists {
		return nil, "", appErrors.NotFound("merchant not found")
	}
	curr := model.MerchantStatus(merch.Status)
	target, err := model.ValidateLifecycleTransition(model.LifecycleActionActivate, curr)
	if err != nil {
		return nil, curr, err
	}
	merch.Status = string(target)
	return merch, curr, nil
}

func (m *mockService) SuspendMerchant(ctx context.Context, id uuid.UUID, reason string, adminID string, reqID string) (*model.Merchant, model.MerchantStatus, error) {
	merch, exists := m.merchants[id]
	if !exists {
		return nil, "", appErrors.NotFound("merchant not found")
	}
	curr := model.MerchantStatus(merch.Status)
	target, err := model.ValidateLifecycleTransition(model.LifecycleActionSuspend, curr)
	if err != nil {
		return nil, curr, err
	}
	merch.Status = string(target)
	return merch, curr, nil
}

func (m *mockService) ReactivateMerchant(ctx context.Context, id uuid.UUID, reason string, adminID string, reqID string) (*model.Merchant, model.MerchantStatus, error) {
	merch, exists := m.merchants[id]
	if !exists {
		return nil, "", appErrors.NotFound("merchant not found")
	}
	curr := model.MerchantStatus(merch.Status)
	target, err := model.ValidateLifecycleTransition(model.LifecycleActionReactivate, curr)
	if err != nil {
		return nil, curr, err
	}
	merch.Status = string(target)
	return merch, curr, nil
}

func (m *mockService) CreateAppeal(ctx context.Context, merchantID uuid.UUID, reason string) (*model.MerchantAppeal, error) {
	return &model.MerchantAppeal{
		ID:         uuid.New(),
		MerchantID: merchantID,
		Reason:     reason,
		Status:     string(model.MerchantAppealStatusPending),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, nil
}

func (m *mockService) GetAppeals(ctx context.Context, merchantID uuid.UUID) ([]*model.MerchantAppeal, error) {
	return []*model.MerchantAppeal{
		{
			ID:         uuid.New(),
			MerchantID: merchantID,
			Reason:     "test appeal",
			Status:     string(model.MerchantAppealStatusPending),
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		},
	}, nil
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
	if createRes.Merchant.Status != merchantpb.MerchantStatus_PENDING {
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
		PanCardNumber:         "TAX-999",
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
	if updateRes.Merchant.PanCardNumber != "TAX-999" {
		t.Errorf("expected TAX-999, got %s", updateRes.Merchant.PanCardNumber)
	}
	// Verify non-editable fields remain unchanged
	if updateRes.Merchant.FirstName != "John" || updateRes.Merchant.LastName != "Doe" {
		t.Errorf("expected non-editable names to remain unchanged, got %s %s", updateRes.Merchant.FirstName, updateRes.Merchant.LastName)
	}
	if updateRes.Merchant.BusinessEmail != "john@example.com" {
		t.Errorf("expected non-editable email to remain unchanged, got %s", updateRes.Merchant.BusinessEmail)
	}
	if updateRes.Merchant.Status != merchantpb.MerchantStatus_PENDING {
		t.Errorf("expected status to remain PENDING, got %s", updateRes.Merchant.Status)
	}

	// 6b. UpdateMerchant with first_name and last_name
	updateNamesRes, err := client.UpdateMerchant(ctx, &merchantpb.UpdateMerchantRequest{
		Id:            merchantID,
		BusinessName:  "Best Merchant Updated",
		FirstName:     "Johnny",
		LastName:      "Depp",
		BusinessPhone: "+19876543210",
		PanCardNumber: "TAX-999",
	})
	if err != nil {
		t.Fatalf("UpdateMerchant with names failed: %v", err)
	}
	if updateNamesRes.Merchant.FirstName != "Johnny" || updateNamesRes.Merchant.LastName != "Depp" {
		t.Errorf("expected updated names Johnny Depp, got %s %s", updateNamesRes.Merchant.FirstName, updateNamesRes.Merchant.LastName)
	}

	// 7. UpdateMerchant with non-existent UUID returns NotFound
	_, err = client.UpdateMerchant(ctx, &merchantpb.UpdateMerchantRequest{
		Id:            uuid.New().String(),
		BusinessName:  "NonExistent",
		BusinessPhone: "+1234567890",
		PanCardNumber:         "TAX-001",
	})
	if err == nil || status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound for non-existent merchant on update, got %v", err)
	}

	// 8. UpdateMerchantStatus
	statusRes, err := client.UpdateMerchantStatus(ctx, &merchantpb.UpdateMerchantStatusRequest{
		Id:     merchantID,
		Status: merchantpb.MerchantStatus_APPROVED,
	})
	if err != nil {
		t.Fatalf("UpdateMerchantStatus failed: %v", err)
	}
	if statusRes.Merchant.Status != merchantpb.MerchantStatus_APPROVED {
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

func TestMerchantGRPC_LifecycleOperations(t *testing.T) {
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

	// Create a test merchant in PENDING state
	mID := uuid.New()
	svc.merchants[mID] = &model.Merchant{
		ID:            mID,
		BusinessName:  "Lifecycle Shop",
		BusinessEmail: "shop@lifecycle.com",
		Status:        string(model.MerchantStatusPending),
	}

	adminCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-user-id", "admin-123",
		"x-user-roles", "ROLE_ADMIN",
		"x-request-id", "req-test-1",
	))

	merchantCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-user-id", "merchant-999",
		"x-user-roles", "ROLE_MERCHANT",
	))

	unauthCtx := context.Background()

	// 1. RBAC Test: Calling Activate with ROLE_MERCHANT -> 403 PermissionDenied
	_, err = client.ActivateMerchant(merchantCtx, &merchantpb.LifecycleMerchantRequest{
		Id:     mID.String(),
		Reason: "Activate try",
	})
	if err == nil || status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied for ROLE_MERCHANT, got: %v", err)
	}

	// 2. Auth Test: Calling Activate without auth headers -> 401 Unauthenticated
	_, err = client.ActivateMerchant(unauthCtx, &merchantpb.LifecycleMerchantRequest{
		Id:     mID.String(),
		Reason: "Activate try",
	})
	if err == nil || status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated for missing auth, got: %v", err)
	}

	// 3. Validation: Missing ID -> 400 InvalidArgument
	_, err = client.ActivateMerchant(adminCtx, &merchantpb.LifecycleMerchantRequest{
		Id: "",
	})
	if err == nil || status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for empty id, got: %v", err)
	}

	// 4. Success: Admin activates merchant (PENDING -> ACTIVE)
	actRes, err := client.ActivateMerchant(adminCtx, &merchantpb.LifecycleMerchantRequest{
		Id:     mID.String(),
		Reason: "Approved documentation",
	})
	if err != nil {
		t.Fatalf("ActivateMerchant failed: %v", err)
	}
	if actRes.Merchant == nil || actRes.Merchant.Id != mID.String() {
		t.Errorf("expected merchant ID %s, got: %v", mID.String(), actRes.Merchant)
	}

	// 5. Conflict Test: Activating already ACTIVE merchant -> 409 Conflict (codes.AlreadyExists)
	_, err = client.ActivateMerchant(adminCtx, &merchantpb.LifecycleMerchantRequest{
		Id:     mID.String(),
		Reason: "Activate again",
	})
	if err == nil || status.Code(err) != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists (409 Conflict) for duplicate activation, got: %v", err)
	}

	// 6. Success: Admin suspends ACTIVE merchant (ACTIVE -> SUSPENDED)
	susRes, err := client.SuspendMerchant(adminCtx, &merchantpb.LifecycleMerchantRequest{
		Id:     mID.String(),
		Reason: "Policy investigation",
	})
	if err != nil {
		t.Fatalf("SuspendMerchant failed: %v", err)
	}
	if susRes.PreviousStatus != merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_ACTIVE {
		t.Errorf("expected previous status ACTIVE, got: %s", susRes.PreviousStatus)
	}

	// 7. Success: Admin reactivates SUSPENDED merchant (SUSPENDED -> ACTIVE)
	reactRes, err := client.ReactivateMerchant(adminCtx, &merchantpb.LifecycleMerchantRequest{
		Id:     mID.String(),
		Reason: "Cleared investigation",
	})
	if err != nil {
		t.Fatalf("ReactivateMerchant failed: %v", err)
	}
	if reactRes.PreviousStatus != merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_SUSPENDED {
		t.Errorf("expected previous status SUSPENDED, got: %s", reactRes.PreviousStatus)
	}
}
