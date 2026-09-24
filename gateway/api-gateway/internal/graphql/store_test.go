package graphql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	storepb "github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	gwGraphQL "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql"
	gwResolvers "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	gatewayGRPC "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"google.golang.org/grpc"
)

type mockStoreClient struct {
	stores map[string]*storepb.Store
}

func newMockStoreClient() *mockStoreClient {
	return &mockStoreClient{
		stores: make(map[string]*storepb.Store),
	}
}

func (m *mockStoreClient) CreateStore(ctx context.Context, in *storepb.CreateStoreRequest, opts ...grpc.CallOption) (*storepb.CreateStoreResponse, error) {
	s := &storepb.Store{
		Id:             "store-1",
		MerchantId:     in.MerchantId,
		Name:           in.Name,
		Slug:           "happy-shop",
		BusinessEmail:  in.BusinessEmail,
		BusinessPhone:  in.BusinessPhone,
		Description:    in.Description,
		LogoUrl:        in.LogoUrl,
		Address:        in.Address,
		IsVacationMode: false,
		ApprovalStatus: "PENDING",
		AvgStoreRating: 0.0,
		CreatedAt:      "2026-09-21T12:00:00Z",
		UpdatedAt:      "2026-09-21T12:00:00Z",
	}
	m.stores[s.Id] = s
	return &storepb.CreateStoreResponse{Store: s}, nil
}

func (m *mockStoreClient) GetStore(ctx context.Context, in *storepb.GetStoreRequest, opts ...grpc.CallOption) (*storepb.GetStoreResponse, error) {
	if in.Id != "" {
		if s, ok := m.stores[in.Id]; ok {
			return &storepb.GetStoreResponse{Store: s}, nil
		}
	}
	for _, s := range m.stores {
		if in.MerchantId != "" && s.MerchantId == in.MerchantId {
			return &storepb.GetStoreResponse{Store: s}, nil
		}
	}
	return &storepb.GetStoreResponse{
		Store: &storepb.Store{
			Id:             "store-1",
			MerchantId:     "merchant-1",
			Name:           "Happy Shop",
			Slug:           "happy-shop",
			ApprovalStatus: "APPROVED",
			IsVacationMode: false,
			CreatedAt:      "2026-09-21T12:00:00Z",
			UpdatedAt:      "2026-09-21T12:00:00Z",
		},
	}, nil
}

func (m *mockStoreClient) ListStores(ctx context.Context, in *storepb.ListStoresRequest, opts ...grpc.CallOption) (*storepb.ListStoresResponse, error) {
	list := make([]*storepb.Store, 0)
	for _, s := range m.stores {
		list = append(list, s)
	}
	return &storepb.ListStoresResponse{
		Stores: list,
		Total:  int32(len(list)),
	}, nil
}

func (m *mockStoreClient) UpdateStore(ctx context.Context, in *storepb.UpdateStoreRequest, opts ...grpc.CallOption) (*storepb.UpdateStoreResponse, error) {
	name := "Happy Shop Updated"
	if in.Name != nil {
		name = *in.Name
	}
	vacation := false
	if in.IsVacationMode != nil {
		vacation = *in.IsVacationMode
	}
	s := &storepb.Store{
		Id:             in.Id,
		MerchantId:     in.MerchantId,
		Name:           name,
		Slug:           "happy-shop-updated",
		ApprovalStatus: "APPROVED",
		IsVacationMode: vacation,
		CreatedAt:      "2026-09-21T12:00:00Z",
		UpdatedAt:      "2026-09-21T12:00:00Z",
	}
	return &storepb.UpdateStoreResponse{Store: s}, nil
}

func (m *mockStoreClient) GetUploadUrl(ctx context.Context, in *storepb.GetUploadUrlRequest, opts ...grpc.CallOption) (*storepb.GetUploadUrlResponse, error) {
	return &storepb.GetUploadUrlResponse{
		UploadUrl:        "https://cljkfzbiywvhzpmlbbuy.storage.supabase.co/storage/v1/s3/logos/test.png?sig=123",
		PublicUrl:        "https://cljkfzbiywvhzpmlbbuy.supabase.co/storage/v1/object/public/stores/logos/test.png",
		Key:              "logos/test.png",
		ExpiresInSeconds: 900,
	}, nil
}

func (m *mockStoreClient) SubmitStore(ctx context.Context, in *storepb.SubmitStoreRequest, opts ...grpc.CallOption) (*storepb.SubmitStoreResponse, error) {
	s, ok := m.stores[in.StoreId]
	if !ok {
		return nil, appErrors.NotFound("store not found")
	}
	s.ApprovalStatus = "PENDING_APPROVAL"
	return &storepb.SubmitStoreResponse{Store: s}, nil
}

func (m *mockStoreClient) ApproveStore(ctx context.Context, in *storepb.ApproveStoreRequest, opts ...grpc.CallOption) (*storepb.ApproveStoreResponse, error) {
	s, ok := m.stores[in.StoreId]
	if !ok {
		return nil, appErrors.NotFound("store not found")
	}
	s.ApprovalStatus = "APPROVED"
	return &storepb.ApproveStoreResponse{Store: s}, nil
}

func (m *mockStoreClient) RejectStore(ctx context.Context, in *storepb.RejectStoreRequest, opts ...grpc.CallOption) (*storepb.RejectStoreResponse, error) {
	s, ok := m.stores[in.StoreId]
	if !ok {
		return nil, appErrors.NotFound("store not found")
	}
	s.ApprovalStatus = "REJECTED"
	s.RejectionReason = in.RejectionReason
	return &storepb.RejectStoreResponse{Store: s}, nil
}

func setupStoreTestApp(t *testing.T) (*fiber.App, string) {
	cfg := &config.Config{
		App: config.AppConfig{
			Version: "1.0.0",
		},
		GraphQL: config.GraphQLConfig{
			IntrospectionEnabled: true,
			PlaygroundEnabled:    true,
		},
		JWT: config.JWTConfig{
			Secret: "test-secret-key-12345",
		},
	}

	storeMock := newMockStoreClient()
	clients := gatewayGRPC.NewClientsWithServices(nil, storeMock)
	resolver := gwResolvers.NewResolver(clients, cfg.App.Version)
	schema, err := gwGraphQL.NewSchema(resolver)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	app := fiber.New()
	handler := gwGraphQL.NewHandler(schema, cfg)
	handler.RegisterRoutes(app)
	legacyServer := gwGraphQL.NewServer(resolver)
	app.All("/graphql", adaptor.HTTPHandler(legacyServer))

	token, _ := auth.GenerateToken(auth.UserContext{
		UserID: "merchant-1",
		Role:   "MERCHANT",
	}, "test-secret-key-12345", 3600*1000000000)

	return app, token
}

func TestStoreGraphQL_CreateStore(t *testing.T) {
	app, token := setupStoreTestApp(t)

	query := `
		mutation {
			createStore(input: {
				name: "Happy Shop"
				description: "Best deals online"
				address: "123 Main St"
			}) {
				id
				name
				slug
				merchantId
				approvalStatus
			}
		}
	`
	reqBody, _ := json.Marshal(map[string]interface{}{
		"query": query,
	})

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data, ok := res["data"].(map[string]interface{})
	if !ok || data["createStore"] == nil {
		t.Fatalf("expected createStore data in response, got: %+v", res)
	}
	store := data["createStore"].(map[string]interface{})
	if store["name"] != "Happy Shop" || store["merchantId"] != "merchant-1" {
		t.Errorf("unexpected createStore response: %+v", store)
	}
}

func TestStoreGraphQL_UpdateStore(t *testing.T) {
	app, token := setupStoreTestApp(t)

	query := `
		mutation {
			updateStore(input: {
				id: "store-1"
				name: "Happy Shop Updated"
				isVacationMode: true
			}) {
				id
				name
				isVacationMode
			}
		}
	`
	reqBody, _ := json.Marshal(map[string]interface{}{
		"query": query,
	})

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data, ok := res["data"].(map[string]interface{})
	if !ok || data["updateStore"] == nil {
		t.Fatalf("expected updateStore data, got: %+v", res)
	}
	store := data["updateStore"].(map[string]interface{})
	if store["name"] != "Happy Shop Updated" {
		t.Errorf("unexpected updateStore name: %v", store["name"])
	}
}

func TestStoreGraphQL_GetStoreAndMyStore(t *testing.T) {
	app, token := setupStoreTestApp(t)

	// MyStore query
	query := `
		query {
			myStore {
				id
				name
				merchantId
				approvalStatus
			}
			store(id: "store-1") {
				id
				name
			}
		}
	`
	reqBody, _ := json.Marshal(map[string]interface{}{
		"query": query,
	})

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data, ok := res["data"].(map[string]interface{})
	if !ok || data["myStore"] == nil || data["store"] == nil {
		t.Fatalf("expected myStore and store queries, got: %+v", res)
	}
}

func TestStoreGraphQL_Unauthenticated_Forbidden(t *testing.T) {
	app, _ := setupStoreTestApp(t)

	query := `
		mutation {
			createStore(input: {
				name: "Unauthorized Store"
			}) {
				id
			}
		}
	`
	reqBody, _ := json.Marshal(map[string]interface{}{
		"query": query,
	})

	// Without Authorization Header
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	errors, ok := res["errors"].([]interface{})
	if !ok || len(errors) == 0 {
		t.Fatalf("expected authorization errors for unauthenticated request, got: %+v", res)
	}
}

func TestStoreGraphQL_GenerateUploadURL(t *testing.T) {
	app, token := setupStoreTestApp(t)

	query := `
		mutation {
			generateStoreUploadUrl(input: {
				imageType: "logo"
				filename: "store-logo.png"
				contentType: "image/png"
			}) {
				uploadUrl
				publicUrl
				key
				expiresInSeconds
			}
		}
	`
	reqBody, _ := json.Marshal(map[string]interface{}{
		"query": query,
	})

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data, ok := res["data"].(map[string]interface{})
	if !ok || data["generateStoreUploadUrl"] == nil {
		t.Fatalf("expected generateStoreUploadUrl data, got: %+v", res)
	}

	payload := data["generateStoreUploadUrl"].(map[string]interface{})
	if payload["uploadUrl"] == "" || payload["publicUrl"] == "" {
		t.Errorf("invalid payload: %+v", payload)
	}
}

func TestStoreGraphQL_SubmitStore(t *testing.T) {
	app, token := setupStoreTestApp(t)

	// Create store first
	createQuery := `
		mutation {
			createStore(input: {
				name: "Happy Shop"
			}) {
				id
				approvalStatus
			}
		}
	`
	reqBody, _ := json.Marshal(map[string]interface{}{"query": createQuery})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	_, _ = app.Test(req)

	// Submit store for approval
	submitQuery := `
		mutation {
			submitStore(id: "store-1") {
				id
				approvalStatus
			}
		}
	`
	reqBody, _ = json.Marshal(map[string]interface{}{"query": submitQuery})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data, ok := res["data"].(map[string]interface{})
	if !ok || data["submitStore"] == nil {
		t.Fatalf("expected submitStore data, got: %+v", res)
	}

	submitted := data["submitStore"].(map[string]interface{})
	if submitted["approvalStatus"] != "PENDING_APPROVAL" {
		t.Errorf("expected PENDING_APPROVAL, got %v", submitted["approvalStatus"])
	}
}

func TestStoreGraphQL_ApproveStore(t *testing.T) {
	app, _ := setupStoreTestApp(t)

	adminToken, _ := auth.GenerateToken(auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	}, "test-secret-key-12345", 3600*1000000000)

	merchantToken, _ := auth.GenerateToken(auth.UserContext{
		UserID: "merchant-1",
		Role:   "MERCHANT",
	}, "test-secret-key-12345", 3600*1000000000)

	// Create store
	createQuery := `mutation { createStore(input: { name: "Happy Shop" }) { id } }`
	reqBody, _ := json.Marshal(map[string]interface{}{"query": createQuery})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+merchantToken)
	_, _ = app.Test(req)

	// Approve store with Admin token
	approveQuery := `
		mutation {
			approveStore(id: "store-1") {
				id
				approvalStatus
			}
		}
	`
	reqBody, _ = json.Marshal(map[string]interface{}{"query": approveQuery})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data, ok := res["data"].(map[string]interface{})
	if !ok || data["approveStore"] == nil {
		t.Fatalf("expected approveStore data, got: %+v", res)
	}

	approved := data["approveStore"].(map[string]interface{})
	if approved["approvalStatus"] != "APPROVED" {
		t.Errorf("expected APPROVED, got %v", approved["approvalStatus"])
	}
}

func TestStoreGraphQL_RejectStore(t *testing.T) {
	app, _ := setupStoreTestApp(t)

	adminToken, _ := auth.GenerateToken(auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	}, "test-secret-key-12345", 3600*1000000000)

	merchantToken, _ := auth.GenerateToken(auth.UserContext{
		UserID: "merchant-1",
		Role:   "MERCHANT",
	}, "test-secret-key-12345", 3600*1000000000)

	// Create store
	createQuery := `mutation { createStore(input: { name: "Happy Shop" }) { id } }`
	reqBody, _ := json.Marshal(map[string]interface{}{"query": createQuery})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+merchantToken)
	_, _ = app.Test(req)

	// Reject store with Admin token
	rejectQuery := `
		mutation {
			rejectStore(id: "store-1", reason: "Invalid bank account details") {
				id
				approvalStatus
				rejectionReason
			}
		}
	`
	reqBody, _ = json.Marshal(map[string]interface{}{"query": rejectQuery})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data, ok := res["data"].(map[string]interface{})
	if !ok || data["rejectStore"] == nil {
		t.Fatalf("expected rejectStore data, got: %+v", res)
	}

	rejected := data["rejectStore"].(map[string]interface{})
	if rejected["approvalStatus"] != "REJECTED" {
		t.Errorf("expected REJECTED, got %v", rejected["approvalStatus"])
	}
	if rejected["rejectionReason"] != "Invalid bank account details" {
		t.Errorf("expected rejection reason to match, got %v", rejected["rejectionReason"])
	}
}

func TestStoreGraphQL_CreateStore_MultipartUpload(t *testing.T) {
	app, token := setupStoreTestApp(t)

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	operations := `{"query":"mutation($input: CreateStoreInput!) { createStore(input: $input) { id name logoUrl } }","variables":{"input":{"name":"Multipart Store","logo":null}}}`
	_ = w.WriteField("operations", operations)
	_ = w.WriteField("map", `{"0":["variables.input.logo"]}`)

	part, _ := w.CreateFormFile("0", "logo.png")
	_, _ = part.Write([]byte("fake-png-binary-content"))
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/graphql", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data, ok := res["data"].(map[string]interface{})
	if !ok || data["createStore"] == nil {
		t.Fatalf("expected createStore data, got: %+v", res)
	}

	store := data["createStore"].(map[string]interface{})
	if store["name"] != "Multipart Store" {
		t.Errorf("expected Multipart Store, got %v", store["name"])
	}
}

