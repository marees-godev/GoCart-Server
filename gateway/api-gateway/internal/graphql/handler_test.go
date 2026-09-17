package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	gwResolver "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	gatewayGRPC "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"
	"google.golang.org/grpc"
)

type mockUserClient struct{}

func (m *mockUserClient) GetUser(ctx context.Context, in *userpb.GetUserRequest, opts ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	return &userpb.GetUserResponse{
		User: &userpb.User{
			Id:        in.Id,
			Email:     "user@example.com",
			FirstName: "Jane",
			LastName:  "Doe",
			Role:      "customer",
			CreatedAt: "2026-01-01T00:00:00Z",
		},
	}, nil
}

func (m *mockUserClient) Login(ctx context.Context, in *userpb.LoginRequest, opts ...grpc.CallOption) (*userpb.AuthResponse, error) {
	return &userpb.AuthResponse{
		Token: "jwt-mock-token",
		User: &userpb.User{
			Id:    "u-100",
			Email: in.Email,
			Role:  "customer",
		},
	}, nil
}

func (m *mockUserClient) Register(ctx context.Context, in *userpb.RegisterRequest, opts ...grpc.CallOption) (*userpb.AuthResponse, error) {
	return &userpb.AuthResponse{
		Token: "jwt-mock-register-token",
		User: &userpb.User{
			Id:        "u-101",
			Email:     in.Email,
			FirstName: in.FirstName,
			LastName:  in.LastName,
			Role:      "customer",
		},
	}, nil
}

// ----------------------------------------------------------------------------
// Helper to Setup Test Fiber App
// ----------------------------------------------------------------------------

func setupTestApp(introEnabled bool) *fiber.App {
	clients := gatewayGRPC.NewClientsWithServices(
		&mockUserClient{},
	)

	resolver := gwResolver.NewResolver(clients)
	schema, _ := NewSchema(resolver)

	cfg := &config.Config{
		GraphQLIntrospectionEnabled: introEnabled,
	}

	handler := NewHandler(schema, cfg)

	app := fiber.New()
	handler.RegisterRoutes(app)
	return app
}

// ----------------------------------------------------------------------------
// Test Cases
// ----------------------------------------------------------------------------

func TestHandleQuery_ValidProductQuery(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `query { user(id: "u-100") { id email firstName } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data, ok := res["data"].(map[string]interface{})
	if !ok || data == nil {
		t.Fatalf("expected data field in response, got %v", res)
	}

	user, ok := data["user"].(map[string]interface{})
	if !ok || user == nil {
		t.Fatalf("expected user object in data, got %v", data)
	}

	if user["id"] != "u-100" {
		t.Errorf("expected user id 'u-100', got '%v'", user["id"])
	}
}

func TestHandleMutation_Login(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `mutation { login(input: {email: "admin@gocart.com", password: "password123"}) { token user { id email } } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data := res["data"].(map[string]interface{})
	login := data["login"].(map[string]interface{})

	if login["token"] != "jwt-mock-token" {
		t.Errorf("expected token 'jwt-mock-token', got '%v'", login["token"])
	}
}

func TestHandleMutation_Register(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `mutation { register(input: {email: "new@gocart.com", password: "password123", firstName: "Jane"}) { token user { id email } } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data := res["data"].(map[string]interface{})
	reg := data["register"].(map[string]interface{})

	if reg["token"] != "jwt-mock-register-token" {
		t.Errorf("expected token 'jwt-mock-register-token', got '%v'", reg["token"])
	}
}

func TestHandleQuery_InvalidGraphQLSyntax(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `query { invalidField { `,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	errors, ok := res["errors"].([]interface{})
	if !ok || len(errors) == 0 {
		t.Fatalf("expected errors array for syntax error, got %v", res)
	}
}

func TestHandleQuery_IntrospectionEnabled(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `query { __schema { queryType { name } } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data, ok := res["data"].(map[string]interface{})
	if !ok || data == nil {
		t.Fatalf("expected data for enabled introspection, got %v", res)
	}
}

func TestHandleQuery_IntrospectionDisabled(t *testing.T) {
	app := setupTestApp(false)

	reqBody := map[string]interface{}{
		"query": `query { __schema { queryType { name } } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	errs, ok := res["errors"].([]interface{})
	if !ok || len(errs) == 0 {
		t.Fatalf("expected error when introspection disabled, got %v", res)
	}

	firstErr := errs[0].(map[string]interface{})
	if firstErr["message"] != "GraphQL introspection is disabled" {
		t.Errorf("expected message 'GraphQL introspection is disabled', got '%v'", firstErr["message"])
	}
}

func TestHandlePlayground_Enabled(t *testing.T) {
	app := setupTestApp(true)

	req := httptest.NewRequest(http.MethodGet, "/playground", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandlePlayground_Disabled(t *testing.T) {
	app := setupTestApp(false)

	req := httptest.NewRequest(http.MethodGet, "/playground", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", resp.StatusCode)
	}
}
