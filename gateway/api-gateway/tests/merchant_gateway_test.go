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
	merchants map[string]*merchantpb.MerchantResponseData
}

func newMockMerchantBackend() *mockMerchantBackend {
	return &mockMerchantBackend{
		merchants: make(map[string]*merchantpb.MerchantResponseData),
	}
}

func (m *mockMerchantBackend) CreateMerchant(ctx context.Context, req *merchantpb.CreateMerchantRequest) (*merchantpb.CreateMerchantResponse, error) {
	id := req.Id
	if id == "" {
		id = uuid.New().String()
	}
	merch := &merchantpb.MerchantResponseData{
		Id:            id,
		BusinessName:  "",
		BusinessEmail: req.BusinessEmail,
		BusinessPhone: "",
		TaxId:         "",
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		Status:        "PENDING",
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
		if req.Status == "" || merch.Status == req.Status {
			list = append(list, merch)
		}
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
	if req.BusinessPhone != "" {
		merch.BusinessPhone = req.BusinessPhone
	}
	if req.TaxId != "" {
		merch.TaxId = req.TaxId
	}
	merch.UpdatedAt = timestamppb.Now()
	return &merchantpb.UpdateMerchantResponse{Merchant: merch}, nil
}

func (m *mockMerchantBackend) UpdateMerchantStatus(ctx context.Context, req *merchantpb.UpdateMerchantStatusRequest) (*merchantpb.UpdateMerchantStatusResponse, error) {
	merch, exists := m.merchants[req.Id]
	if !exists {
		return nil, status.Error(codes.NotFound, "merchant not found")
	}
	merch.Status = req.Status
	merch.RejectionReason = req.RejectionReason
	merch.UpdatedAt = timestamppb.Now()
	return &merchantpb.UpdateMerchantStatusResponse{Merchant: merch}, nil
}

func (m *mockMerchantBackend) DeleteMerchant(ctx context.Context, req *merchantpb.DeleteMerchantRequest) (*merchantpb.DeleteMerchantResponse, error) {
	if _, exists := m.merchants[req.Id]; !exists {
		return nil, status.Error(codes.NotFound, "merchant not found")
	}
	delete(m.merchants, req.Id)
	return &merchantpb.DeleteMerchantResponse{Success: true}, nil
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
		Status:       "PENDING",
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
			updateMerchantStatus(id: "%s", status: "APPROVED") {
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
				taxId: "TAX-123"
			}) {
				id
				businessName
				businessPhone
				taxId
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
				TaxID         *string `json:"taxId"`
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
	if updateRes.Data.UpdateMerchant.TaxID == nil || *updateRes.Data.UpdateMerchant.TaxID != "TAX-123" {
		t.Errorf("expected TAX-123, got %v", updateRes.Data.UpdateMerchant.TaxID)
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
	suspendMutation := fmt.Sprintf(`mutation { updateMerchantStatus(id: "%s", status: "SUSPENDED", rejectionReason: "Suspicious activity") { id status } }`, merchantID)
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
		Status:       "PENDING",
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
	statusMutation := `mutation { updateMerchantStatus(id: "m-1", status: "APPROVED") { id status } }`
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
