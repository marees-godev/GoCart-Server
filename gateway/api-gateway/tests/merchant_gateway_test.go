package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/client"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	gwGraphQL "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql"
	gwResolver "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockMerchantBackend struct {
	merchantpb.UnimplementedMerchantServiceServer
	merchants   map[string]*merchantpb.MerchantResponseData
	appeals     map[string][]*merchantpb.MerchantAppealData
	reactivated map[string]bool
}

func newMockMerchantBackend() *mockMerchantBackend {
	return &mockMerchantBackend{
		merchants:   make(map[string]*merchantpb.MerchantResponseData),
		appeals:     make(map[string][]*merchantpb.MerchantAppealData),
		reactivated: make(map[string]bool),
	}
}

func (m *mockMerchantBackend) CreateMerchant(ctx context.Context, req *merchantpb.CreateMerchantRequest) (*merchantpb.CreateMerchantResponse, error) {
	id := req.Id
	if id == "" {
		id = uuid.New().String()
	}
	merch := &merchantpb.MerchantResponseData{
		Id:            id,
		BusinessEmail: req.BusinessEmail,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		Status:        merchantpb.MerchantStatus_PENDING,
		CreatedAt:     timestamppb.Now(),
		UpdatedAt:     timestamppb.Now(),
	}
	m.merchants[id] = merch
	return &merchantpb.CreateMerchantResponse{Merchant: merch}, nil
}

func (m *mockMerchantBackend) GetMerchant(ctx context.Context, req *merchantpb.GetMerchantRequest) (*merchantpb.GetMerchantResponse, error) {
	merch, exists := m.merchants[req.Id]
	if !exists {
		return nil, status.Error(codes.NotFound, "merchant not found")
	}
	return &merchantpb.GetMerchantResponse{Merchant: merch}, nil
}

func (m *mockMerchantBackend) ListMerchants(ctx context.Context, req *merchantpb.ListMerchantsRequest) (*merchantpb.ListMerchantsResponse, error) {
	var list []*merchantpb.MerchantResponseData
	for _, merch := range m.merchants {
		if req.ReactivatedOnly != nil && *req.ReactivatedOnly {
			if !m.reactivated[merch.Id] {
				continue
			}
		}
		if req.Status != nil && merch.Status != *req.Status {
			continue
		}
		if req.LifecycleStatus != nil && *req.LifecycleStatus != merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_UNSPECIFIED {
			ls := *req.LifecycleStatus
			if ls == merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_ACTIVE && merch.Status != merchantpb.MerchantStatus_APPROVED {
				continue
			}
			if ls == merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_SUSPENDED && merch.Status != merchantpb.MerchantStatus_SUSPENDED {
				continue
			}
		}
		list = append(list, merch)
	}
	return &merchantpb.ListMerchantsResponse{
		Merchants: list,
		Total:     int32(len(list)),
	}, nil
}

func (m *mockMerchantBackend) UpdateMerchant(ctx context.Context, req *merchantpb.UpdateMerchantRequest) (*merchantpb.UpdateMerchantResponse, error) {
	merch, exists := m.merchants[req.Id]
	if !exists {
		return nil, status.Error(codes.NotFound, "merchant not found")
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
	if req.BusinessPhone != "" {
		merch.BusinessPhone = req.BusinessPhone
	}
	if req.PanCardNumber != "" {
		merch.PanCardNumber = req.PanCardNumber
	}
	merch.UpdatedAt = timestamppb.Now()
	return &merchantpb.UpdateMerchantResponse{Merchant: merch}, nil
}

func (m *mockMerchantBackend) UpdateMerchantStatus(ctx context.Context, req *merchantpb.UpdateMerchantStatusRequest) (*merchantpb.UpdateMerchantStatusResponse, error) {
	merch, exists := m.merchants[req.Id]
	if !exists {
		return nil, status.Error(codes.NotFound, "merchant not found")
	}
	prev := merch.Status
	merch.Status = req.Status
	merch.RejectionReason = req.RejectionReason
	merch.UpdatedAt = timestamppb.Now()
	if req.Status == merchantpb.MerchantStatus_APPROVED && (len(m.appeals[req.Id]) > 0 || prev == merchantpb.MerchantStatus_SUSPENDED) {
		m.reactivated[req.Id] = true
	}

	return &merchantpb.UpdateMerchantStatusResponse{
		Merchant:       merch,
		PreviousStatus: prev,
	}, nil
}

func (m *mockMerchantBackend) DeleteMerchant(ctx context.Context, req *merchantpb.DeleteMerchantRequest) (*merchantpb.DeleteMerchantResponse, error) {
	if _, exists := m.merchants[req.Id]; !exists {
		return nil, status.Error(codes.NotFound, "merchant not found")
	}
	delete(m.merchants, req.Id)
	return &merchantpb.DeleteMerchantResponse{Success: true}, nil
}

func (m *mockMerchantBackend) ActivateMerchant(ctx context.Context, req *merchantpb.LifecycleMerchantRequest) (*merchantpb.LifecycleMerchantResponse, error) {
	merch, exists := m.merchants[req.Id]
	if !exists {
		return nil, status.Error(codes.NotFound, "merchant not found")
	}
	if merch.Status == merchantpb.MerchantStatus_APPROVED {
		return nil, status.Error(codes.FailedPrecondition, "cannot transition to ACTIVE from current status")
	}
	prev := merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_INACTIVE
	if merch.Status == merchantpb.MerchantStatus_SUSPENDED {
		prev = merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_SUSPENDED
	}
	merch.Status = merchantpb.MerchantStatus_APPROVED
	merch.UpdatedAt = timestamppb.Now()
	return &merchantpb.LifecycleMerchantResponse{
		Merchant:       merch,
		PreviousStatus: prev,
		Message:        "merchant activated successfully",
	}, nil
}

func (m *mockMerchantBackend) SuspendMerchant(ctx context.Context, req *merchantpb.LifecycleMerchantRequest) (*merchantpb.LifecycleMerchantResponse, error) {
	merch, exists := m.merchants[req.Id]
	if !exists {
		return nil, status.Error(codes.NotFound, "merchant not found")
	}
	if merch.Status == merchantpb.MerchantStatus_SUSPENDED {
		return nil, status.Error(codes.FailedPrecondition, "cannot transition to SUSPENDED from current status")
	}
	prev := merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_ACTIVE
	merch.Status = merchantpb.MerchantStatus_SUSPENDED
	merch.UpdatedAt = timestamppb.Now()
	return &merchantpb.LifecycleMerchantResponse{
		Merchant:       merch,
		PreviousStatus: prev,
		Message:        "merchant suspended successfully",
	}, nil
}

func (m *mockMerchantBackend) ReactivateMerchant(ctx context.Context, req *merchantpb.LifecycleMerchantRequest) (*merchantpb.LifecycleMerchantResponse, error) {
	merch, exists := m.merchants[req.Id]
	if !exists {
		return nil, status.Error(codes.NotFound, "merchant not found")
	}
	if merch.Status != merchantpb.MerchantStatus_SUSPENDED {
		return nil, status.Error(codes.FailedPrecondition, "cannot transition to ACTIVE because merchant is not SUSPENDED")
	}
	prev := merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_SUSPENDED
	merch.Status = merchantpb.MerchantStatus_APPROVED
	merch.UpdatedAt = timestamppb.Now()
	m.reactivated[req.Id] = true
	return &merchantpb.LifecycleMerchantResponse{
		Merchant:       merch,
		PreviousStatus: prev,
		Message:        "merchant reactivated successfully",
	}, nil
}

func (m *mockMerchantBackend) CreateMerchantAppeal(ctx context.Context, req *merchantpb.CreateMerchantAppealRequest) (*merchantpb.CreateMerchantAppealResponse, error) {
	merch, exists := m.merchants[req.MerchantId]
	if !exists {
		return nil, status.Error(codes.NotFound, "merchant not found")
	}
	if merch.Status != merchantpb.MerchantStatus_SUSPENDED {
		return nil, status.Error(codes.FailedPrecondition, "appeal can only be submitted for a suspended merchant")
	}
	appeal := &merchantpb.MerchantAppealData{
		Id:         "appeal-" + uuid.New().String(),
		MerchantId: req.MerchantId,
		Reason:     req.Reason,
		Status:     "PENDING",
		CreatedAt:  timestamppb.Now(),
		UpdatedAt:  timestamppb.Now(),
	}
	if m.appeals == nil {
		m.appeals = make(map[string][]*merchantpb.MerchantAppealData)
	}
	m.appeals[req.MerchantId] = append(m.appeals[req.MerchantId], appeal)

	merch.Status = merchantpb.MerchantStatus_PENDING
	merch.UpdatedAt = timestamppb.Now()
	return &merchantpb.CreateMerchantAppealResponse{Appeal: appeal}, nil
}

func (m *mockMerchantBackend) GetMerchantAppeals(ctx context.Context, req *merchantpb.GetMerchantAppealsRequest) (*merchantpb.GetMerchantAppealsResponse, error) {
	return &merchantpb.GetMerchantAppealsResponse{
		Appeals: m.appeals[req.MerchantId],
	}, nil
}

func setupMerchantGatewayTest(t *testing.T, backend *mockMerchantBackend) (*fiber.App, func(), string) {
	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	merchantpb.RegisterMerchantServiceServer(grpcServer, backend)

	go func() {
		_ = grpcServer.Serve(lis)
	}()

	cfg := &config.Config{
		App: config.AppConfig{
			Name:        "api-gateway",
			Environment: "test",
			Version:     "1.0.0",
		},
		GRPC: config.GRPCConfig{
			MerchantServiceAddr: "passthrough://bufnet",
			DefaultTimeout:      2 * time.Second,
		},
		JWT: config.JWTConfig{
			Secret: "test-secret-key-12345",
		},
		GraphQLIntrospectionEnabled: true,
	}

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	clientMgr, err := client.NewClientManager(cfg, grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("failed to create client manager: %v", err)
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	gqlResolver := gwResolver.NewResolverWithManager(nil, clientMgr, "1.0.0")
	gqlSchema, err := gwGraphQL.NewSchema(gqlResolver)
	if err != nil {
		t.Fatalf("failed to init GraphQL schema: %v", err)
	}

	gqlHandler := gwGraphQL.NewHandler(gqlSchema, cfg)
	gqlHandler.RegisterRoutes(app)

	token, _ := auth.GenerateToken(auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	}, "test-secret-key-12345", time.Hour)

	cleanup := func() {
		clientMgr.Close()
		grpcServer.Stop()
		_ = lis.Close()
	}

	return app, cleanup, token
}

func TestE2E_GraphQLMerchant_CRUD(t *testing.T) {
	backend := newMockMerchantBackend()
	app, cleanup, token := setupMerchantGatewayTest(t, backend)
	defer cleanup()

	// 1. Verify createMerchant mutation is not on API Gateway (must use auth service)
	createMutation := `
		mutation {
			createMerchant(input: {
				userId: "user-100"
				businessName: "Acme Retail"
			}) {
				id
			}
		}
	`
	reqBody, _ := json.Marshal(map[string]string{"query": createMutation})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("createMerchant request failed: %v", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(bodyBytes, []byte("Cannot query field")) && !bytes.Contains(bodyBytes, []byte("errors")) {
		t.Fatalf("expected createMerchant to be rejected by GraphQL schema, got: %s", string(bodyBytes))
	}

	// Seed merchant in mock backend (merchant accounts are provisioned via auth-service)
	merchantID := "m-100"
	backend.merchants[merchantID] = &merchantpb.MerchantResponseData{
		Id:           merchantID,
		BusinessName: "Acme Retail",
		FirstName:    "Alice",
		LastName:     "Smith",
		Status:       merchantpb.MerchantStatus_PENDING,
		CreatedAt:    timestamppb.Now(),
		UpdatedAt:    timestamppb.Now(),
	}

	// 2. Query Merchant by ID
	queryByID := fmt.Sprintf(`query { merchant(id: "%s") { id businessName status } }`, merchantID)
	reqBody, _ = json.Marshal(map[string]string{"query": queryByID})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("merchant query failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	var getRes struct {
		Data struct {
			Merchant struct {
				ID           string `json:"id"`
				BusinessName string `json:"businessName"`
			} `json:"merchant"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &getRes)
	if len(getRes.Errors) > 0 {
		t.Fatalf("GraphQL errors in get: %v", getRes.Errors)
	}
	if getRes.Data.Merchant.ID != merchantID {
		t.Errorf("expected id %s, got %s", merchantID, getRes.Data.Merchant.ID)
	}

	// 3. UpdateMerchantStatus Mutation
	statusMutation := fmt.Sprintf(`
		mutation {
			updateMerchantStatus(id: "%s", status: APPROVED) {
				id
				status
			}
		}
	`, merchantID)
	reqBody, _ = json.Marshal(map[string]string{"query": statusMutation})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("updateMerchantStatus request failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	var statusResult struct {
		Data struct {
			UpdateMerchantStatus struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"updateMerchantStatus"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &statusResult)
	if len(statusResult.Errors) > 0 {
		t.Fatalf("GraphQL errors in updateStatus: %v", statusResult.Errors)
	}
	if statusResult.Data.UpdateMerchantStatus.Status != "APPROVED" {
		t.Errorf("expected APPROVED, got %s", statusResult.Data.UpdateMerchantStatus.Status)
	}

	// 4. UpdateMerchant Mutation
	updateMutation := fmt.Sprintf(`
		mutation {
			updateMerchant(id: "%s", input: {
				businessName: "Acme Super Store"
				businessPhone: "+1234567890"
				panCardNumber: "ABCDE1234F"
			}) {
				id
				businessName
				businessPhone
				panCardNumber
			}
		}
	`, merchantID)
	reqBody, _ = json.Marshal(map[string]string{"query": updateMutation})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("updateMerchant request failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	var updateRes struct {
		Data struct {
			UpdateMerchant struct {
				ID            string  `json:"id"`
				BusinessName  string  `json:"businessName"`
				BusinessPhone *string `json:"businessPhone"`
				PanCardNumber *string `json:"panCardNumber"`
			} `json:"updateMerchant"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &updateRes)
	if len(updateRes.Errors) > 0 {
		t.Fatalf("GraphQL errors in update: %v", updateRes.Errors)
	}
	if updateRes.Data.UpdateMerchant.BusinessName != "Acme Super Store" {
		t.Errorf("expected Acme Super Store, got %s", updateRes.Data.UpdateMerchant.BusinessName)
	}
	if updateRes.Data.UpdateMerchant.BusinessPhone == nil || *updateRes.Data.UpdateMerchant.BusinessPhone != "+1234567890" {
		t.Errorf("expected +1234567890, got %v", updateRes.Data.UpdateMerchant.BusinessPhone)
	}
	if updateRes.Data.UpdateMerchant.PanCardNumber == nil || *updateRes.Data.UpdateMerchant.PanCardNumber != "ABCDE1234F" {
		t.Errorf("expected ABCDE1234F, got %v", updateRes.Data.UpdateMerchant.PanCardNumber)
	}

	// 5. Query MerchantByUserID -> Rejected by schema (removed)
	queryByUser := fmt.Sprintf(`query { merchantByUserId(userId: "%s") { id businessName } }`, merchantID)
	reqBody, _ = json.Marshal(map[string]string{"query": queryByUser})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("merchantByUserId request failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	if !bytes.Contains(bodyBytes, []byte("Cannot query field")) && !bytes.Contains(bodyBytes, []byte("errors")) {
		t.Fatalf("expected merchantByUserId to be rejected by GraphQL schema, got: %s", string(bodyBytes))
	}

	// 6. Query Merchants List
	queryList := `query { merchants(limit: 10) { total merchants { id businessName } } }`
	reqBody, _ = json.Marshal(map[string]string{"query": queryList})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("merchants query failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	var listRes struct {
		Data struct {
			Merchants struct {
				Total     int `json:"total"`
				Merchants []struct {
					ID string `json:"id"`
				} `json:"merchants"`
			} `json:"merchants"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &listRes)
	if len(listRes.Errors) > 0 {
		t.Fatalf("GraphQL errors in merchants: %v", listRes.Errors)
	}
	if listRes.Data.Merchants.Total != 1 {
		t.Errorf("expected total 1, got %d", listRes.Data.Merchants.Total)
	}

	// 7. DeleteMerchant & Suspend Rules
	// 7a. Admin cannot delete merchant -> FORBIDDEN (admin can only suspend)
	delMutation := fmt.Sprintf(`mutation { deleteMerchant(id: "%s") }`, merchantID)
	reqBody, _ = json.Marshal(map[string]string{"query": delMutation})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("deleteMerchant request failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	var adminDelResult struct {
		Errors []struct {
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &adminDelResult)
	if len(adminDelResult.Errors) == 0 || adminDelResult.Errors[0].Extensions.Code != "FORBIDDEN" {
		t.Fatalf("expected admin deletion to be FORBIDDEN, got: %s", string(bodyBytes))
	}

	// 7b. Admin can suspend the merchant
	suspendMutation := fmt.Sprintf(`mutation { updateMerchantStatus(id: "%s", status: SUSPENDED, rejectionReason: "Suspicious activity") { id status } }`, merchantID)
	reqBody, _ = json.Marshal(map[string]string{"query": suspendMutation})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("suspendMerchant request failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	var suspendResult struct {
		Data struct {
			UpdateMerchantStatus struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"updateMerchantStatus"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &suspendResult)
	if len(suspendResult.Errors) > 0 {
		t.Fatalf("errors in suspend: %v", suspendResult.Errors)
	}
	if suspendResult.Data.UpdateMerchantStatus.Status != "SUSPENDED" {
		t.Errorf("expected SUSPENDED, got %s", suspendResult.Data.UpdateMerchantStatus.Status)
	}

	// 7c. Merchant owner can delete their own account
	ownerMerchantToken, _ := auth.GenerateToken(auth.UserContext{
		UserID: merchantID,
		Role:   "MERCHANT",
	}, "test-secret-key-12345", time.Hour)

	reqBody, _ = json.Marshal(map[string]string{"query": delMutation})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ownerMerchantToken)

	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("owner deleteMerchant request failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	var delResult struct {
		Data struct {
			DeleteMerchant bool `json:"deleteMerchant"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &delResult)
	if len(delResult.Errors) > 0 {
		t.Fatalf("GraphQL errors in owner delete: %v", delResult.Errors)
	}
	if !delResult.Data.DeleteMerchant {
		t.Errorf("expected deleteMerchant true, got false")
	}
}

func TestE2E_GraphQLMerchant_RoleAuthorization(t *testing.T) {
	backend := newMockMerchantBackend()
	backend.merchants["m-1"] = &merchantpb.MerchantResponseData{
		Id:           "m-1",
		BusinessName: "Original Shop",
		Status:       merchantpb.MerchantStatus_PENDING,
		CreatedAt:    timestamppb.Now(),
		UpdatedAt:    timestamppb.Now(),
	}
	app, cleanup, _ := setupMerchantGatewayTest(t, backend)
	defer cleanup()

	updateMutation := `
		mutation {
			updateMerchant(id: "m-1", input: {
				businessName: "Updated Shop"
			}) {
				id
				businessName
			}
		}
	`

	// 1. Unauthenticated -> UNAUTHORIZED
	reqBody, _ := json.Marshal(map[string]string{"query": updateMutation})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("unauthenticated request failed: %v", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	var unauthRes struct {
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &unauthRes)
	if len(unauthRes.Errors) == 0 || unauthRes.Errors[0].Extensions.Code != "UNAUTHORIZED" {
		t.Errorf("expected UNAUTHORIZED, got: %s", string(bodyBytes))
	}

	// 2. Role CUSTOMER -> FORBIDDEN
	customerToken, _ := auth.GenerateToken(auth.UserContext{
		UserID: "customer-1",
		Role:   "CUSTOMER",
	}, "test-secret-key-12345", time.Hour)

	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+customerToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("customer request failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	var forbiddenRes struct {
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &forbiddenRes)
	if len(forbiddenRes.Errors) == 0 || forbiddenRes.Errors[0].Extensions.Code != "FORBIDDEN" {
		t.Errorf("expected FORBIDDEN for CUSTOMER role, got: %s", string(bodyBytes))
	}

	// 3. Role MERCHANT -> SUCCESS
	merchantToken, _ := auth.GenerateToken(auth.UserContext{
		UserID: "m-1",
		Role:   "MERCHANT",
	}, "test-secret-key-12345", time.Hour)

	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+merchantToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("merchant request failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	var successRes struct {
		Data struct {
			UpdateMerchant struct {
				ID           string `json:"id"`
				BusinessName string `json:"businessName"`
			} `json:"updateMerchant"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &successRes)
	if len(successRes.Errors) > 0 {
		t.Fatalf("expected success for MERCHANT role, got errors: %v", successRes.Errors)
	}
	if successRes.Data.UpdateMerchant.BusinessName != "Updated Shop" {
		t.Errorf("expected 'Updated Shop', got %s", successRes.Data.UpdateMerchant.BusinessName)
	}

	// 4. Role MERCHANT updating status -> FORBIDDEN (requires ADMIN)
	statusMutation := `mutation { updateMerchantStatus(id: "m-1", status: APPROVED) { id status } }`
	reqBody, _ = json.Marshal(map[string]string{"query": statusMutation})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+merchantToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("merchant update status failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	var statusForbiddenRes struct {
		Errors []struct {
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &statusForbiddenRes)
	if len(statusForbiddenRes.Errors) == 0 || statusForbiddenRes.Errors[0].Extensions.Code != "FORBIDDEN" {
		t.Errorf("expected FORBIDDEN for MERCHANT updating status, got: %s", string(bodyBytes))
	}

	// 5. Role ADMIN deleting merchant -> FORBIDDEN (admins cannot delete, only suspend)
	adminToken, _ := auth.GenerateToken(auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	}, "test-secret-key-12345", time.Hour)

	delMutation := `mutation { deleteMerchant(id: "m-1") }`
	reqBody, _ = json.Marshal(map[string]string{"query": delMutation})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("admin delete merchant failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	var adminDelForbidden struct {
		Errors []struct {
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &adminDelForbidden)
	if len(adminDelForbidden.Errors) == 0 || adminDelForbidden.Errors[0].Extensions.Code != "FORBIDDEN" {
		t.Errorf("expected FORBIDDEN for ADMIN deleting merchant, got: %s", string(bodyBytes))
	}

	// 6. Role MERCHANT (non-owner) deleting merchant -> FORBIDDEN
	nonOwnerToken, _ := auth.GenerateToken(auth.UserContext{
		UserID: "merchant-other",
		Role:   "MERCHANT",
	}, "test-secret-key-12345", time.Hour)

	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+nonOwnerToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("non-owner delete merchant failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	var nonOwnerForbidden struct {
		Errors []struct {
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &nonOwnerForbidden)
	if len(nonOwnerForbidden.Errors) == 0 || nonOwnerForbidden.Errors[0].Extensions.Code != "FORBIDDEN" {
		t.Errorf("expected FORBIDDEN for non-owner deleting merchant, got: %s", string(bodyBytes))
	}

	// 7. Role MERCHANT (owner) deleting merchant -> SUCCESS
	ownerToken, _ := auth.GenerateToken(auth.UserContext{
		UserID: "m-1",
		Role:   "MERCHANT",
	}, "test-secret-key-12345", time.Hour)

	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("owner delete merchant failed: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	var ownerDelSuccess struct {
		Data struct {
			DeleteMerchant bool `json:"deleteMerchant"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &ownerDelSuccess)
	if len(ownerDelSuccess.Errors) > 0 || !ownerDelSuccess.Data.DeleteMerchant {
		t.Errorf("expected SUCCESS for owner deleting merchant, got: %s", string(bodyBytes))
	}
}

func TestE2E_GraphQLMerchant_Lifecycle(t *testing.T) {
	backend := newMockMerchantBackend()
	app, cleanup, _ := setupMerchantGatewayTest(t, backend)
	defer cleanup()

	jwtSecret := "test-secret-key-12345"

	// Seed a test merchant in PENDING state
	testMerchantID := "merchant-lifecycle-1"
	backend.merchants[testMerchantID] = &merchantpb.MerchantResponseData{
		Id:            testMerchantID,
		BusinessName: "Lifecycle Test Store",
		BusinessEmail: "lifecycle@example.com",
		Status:        merchantpb.MerchantStatus_PENDING,
		CreatedAt:     timestamppb.Now(),
		UpdatedAt:     timestamppb.Now(),
	}

	adminToken, _ := auth.GenerateToken(auth.UserContext{
		UserID: "admin-user",
		Role:   "ADMIN",
	}, jwtSecret, time.Hour)

	merchantToken, _ := auth.GenerateToken(auth.UserContext{
		UserID: testMerchantID,
		Role:   "MERCHANT",
	}, jwtSecret, time.Hour)

	// 1. Unauthenticated caller attempting updateMerchantStatus -> UNAUTHORIZED (401)
	updateStatusMutation := fmt.Sprintf(`{"query":"mutation { updateMerchantStatus(id: \"%s\", status: APPROVED) { id status rejectionReason } }"}`, testMerchantID)
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader([]byte(updateStatusMutation)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("unauth request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	var unauthResp struct {
		Errors []struct {
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(body, &unauthResp)
	if len(unauthResp.Errors) == 0 || unauthResp.Errors[0].Extensions.Code != "UNAUTHORIZED" {
		t.Errorf("expected UNAUTHORIZED for unauthenticated updateMerchantStatus, got: %s", string(body))
	}

	// 2. Standard MERCHANT caller attempting updateMerchantStatus -> FORBIDDEN (403)
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader([]byte(updateStatusMutation)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+merchantToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("merchant role request failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	var merchantResp struct {
		Errors []struct {
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(body, &merchantResp)
	if len(merchantResp.Errors) == 0 || merchantResp.Errors[0].Extensions.Code != "FORBIDDEN" {
		t.Errorf("expected FORBIDDEN for standard merchant role on updateMerchantStatus, got: %s", string(body))
	}

	// 3. Authorized ADMIN approving PENDING merchant -> SUCCESS (status APPROVED)
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader([]byte(updateStatusMutation)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("admin approval request failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	var approveSuccess struct {
		Data struct {
			UpdateMerchantStatus struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"updateMerchantStatus"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(body, &approveSuccess)
	if len(approveSuccess.Errors) > 0 || approveSuccess.Data.UpdateMerchantStatus.Status != "APPROVED" {
		t.Fatalf("expected status APPROVED after admin approval, got: %s", string(body))
	}

	// 4. Query activeMerchants -> returns the approved merchant
	activeQuery := `{"query":"query { activeMerchants { merchants { id businessName status } total } }"}`
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader([]byte(activeQuery)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("query activeMerchants failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	var activeResp struct {
		Data struct {
			ActiveMerchants struct {
				Merchants []struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"merchants"`
				Total int `json:"total"`
			} `json:"activeMerchants"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(body, &activeResp)
	if len(activeResp.Errors) > 0 || activeResp.Data.ActiveMerchants.Total == 0 {
		t.Fatalf("expected activeMerchants to return approved merchant, got: %s", string(body))
	}

	// 5. Authorized ADMIN suspending merchant -> SUCCESS (status SUSPENDED)
	suspendMutation := fmt.Sprintf(`{"query":"mutation { updateMerchantStatus(id: \"%s\", status: SUSPENDED, rejectionReason: \"Compliance review\") { id status rejectionReason } }"}`, testMerchantID)
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader([]byte(suspendMutation)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("admin suspension request failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	var suspendSuccess struct {
		Data struct {
			UpdateMerchantStatus struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"updateMerchantStatus"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(body, &suspendSuccess)
	if len(suspendSuccess.Errors) > 0 || suspendSuccess.Data.UpdateMerchantStatus.Status != "SUSPENDED" {
		t.Fatalf("expected status SUSPENDED after admin suspension, got: %s", string(body))
	}

	// 6. Query suspendedMerchants -> returns the suspended merchant
	suspendedQuery := `{"query":"query { suspendedMerchants { merchants { id businessName status } total } }"}`
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader([]byte(suspendedQuery)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("query suspendedMerchants failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	var suspendedResp struct {
		Data struct {
			SuspendedMerchants struct {
				Merchants []struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"merchants"`
				Total int `json:"total"`
			} `json:"suspendedMerchants"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(body, &suspendedResp)
	if len(suspendedResp.Errors) > 0 || suspendedResp.Data.SuspendedMerchants.Total == 0 {
		t.Fatalf("expected suspendedMerchants to return suspended merchant, got: %s", string(body))
	}

	// 7. Standard merchant submits appeal for SUSPENDED account -> SUCCESS (appeal status PENDING)
	appealMutation := fmt.Sprintf(`{"query":"mutation { merchantAppeal(merchantId: \"%s\", reason: \"Compliance cleared with updated documents\") { id merchantId reason status createdAt updatedAt } }"}`, testMerchantID)
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader([]byte(appealMutation)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+merchantToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("merchant appeal request failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	var appealSuccess struct {
		Data struct {
			MerchantAppeal struct {
				ID         string `json:"id"`
				MerchantID string `json:"merchantId"`
				Reason     string `json:"reason"`
				Status     string `json:"status"`
			} `json:"merchantAppeal"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(body, &appealSuccess)
	if len(appealSuccess.Errors) > 0 || appealSuccess.Data.MerchantAppeal.Status != "PENDING" {
		t.Fatalf("expected successful appeal with status PENDING, got: %s", string(body))
	}

	// 8. Verify merchant status returned to PENDING after appeal
	getMerchantQuery := fmt.Sprintf(`{"query":"query { merchant(id: \"%s\") { id status } }"}`, testMerchantID)
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader([]byte(getMerchantQuery)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("query merchant after appeal failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	var merchantStatusResp struct {
		Data struct {
			Merchant struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"merchant"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(body, &merchantStatusResp)
	if len(merchantStatusResp.Errors) > 0 || merchantStatusResp.Data.Merchant.Status != "PENDING" {
		t.Fatalf("expected merchant status PENDING after appeal, got: %s", string(body))
	}

	// 9. Query merchantAppeals -> returns the submitted appeal
	appealQuery := fmt.Sprintf(`{"query":"query { merchantAppeals(merchantId: \"%s\") { id merchantId reason status } }"}`, testMerchantID)
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader([]byte(appealQuery)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+merchantToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("merchant appeals query failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	var appealsListResp struct {
		Data struct {
			MerchantAppeals []struct {
				ID         string `json:"id"`
				MerchantID string `json:"merchantId"`
			} `json:"merchantAppeals"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(body, &appealsListResp)
	if len(appealsListResp.Errors) > 0 || len(appealsListResp.Data.MerchantAppeals) == 0 {
		t.Fatalf("expected merchant appeals list, got: %s", string(body))
	}

	// 10. Admin approves the re-appealed merchant -> SUCCESS (status APPROVED)
	reactivateMutation := fmt.Sprintf(`{"query":"mutation { updateMerchantStatus(id: \"%s\", status: APPROVED) { id status } }"}`, testMerchantID)
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader([]byte(reactivateMutation)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("admin re-approval request failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	var reactivateSuccess struct {
		Data struct {
			UpdateMerchantStatus struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"updateMerchantStatus"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(body, &reactivateSuccess)
	if len(reactivateSuccess.Errors) > 0 || reactivateSuccess.Data.UpdateMerchantStatus.Status != "APPROVED" {
		t.Fatalf("expected status APPROVED after reactivation, got: %s", string(body))
	}

	// 11. Query reactivatedMerchants -> returns the reactivated merchant
	reactivatedQuery := `{"query":"query { reactivatedMerchants { merchants { id businessName status } total } }"}`
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader([]byte(reactivatedQuery)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("query reactivatedMerchants failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	var reactivatedResp struct {
		Data struct {
			ReactivatedMerchants struct {
				Merchants []struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"merchants"`
				Total int `json:"total"`
			} `json:"reactivatedMerchants"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(body, &reactivatedResp)
	if len(reactivatedResp.Errors) > 0 || reactivatedResp.Data.ReactivatedMerchants.Total == 0 {
		t.Fatalf("expected reactivatedMerchants list, got: %s", string(body))
	}


}

