package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	gwResolver "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	gatewayGRPC "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"google.golang.org/grpc"
)

const testJWTSecret = "gocart-secret-key-change-in-production"

type mockAuthClient struct {
	authpb.AuthServiceClient
}

func (m *mockAuthClient) Login(ctx context.Context, in *authpb.LoginRequest, opts ...grpc.CallOption) (*authpb.AuthResponse, error) {
	return &authpb.AuthResponse{
		AccessToken: "jwt-mock-token",
		UserId:      "u-100",
	}, nil
}

func (m *mockAuthClient) Register(ctx context.Context, in *authpb.RegisterRequest, opts ...grpc.CallOption) (*authpb.AuthResponse, error) {
	return &authpb.AuthResponse{
		AccessToken: "jwt-mock-register-token",
		UserId:      "u-101",
	}, nil
}

type mockUserClient struct {
	userpb.UserServiceClient
}

func (m *mockUserClient) GetUser(ctx context.Context, in *userpb.GetUserRequest, opts ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	return &userpb.GetUserResponse{
		User: &userpb.User{
			Id:        in.Id,
			Email:     "user@example.com",
			FirstName: "Jane",
			LastName:  "Doe",
			CreatedAt: "2026-01-01T00:00:00Z",
		},
	}, nil
}

func (m *mockUserClient) CreateUserAddress(ctx context.Context, in *userpb.CreateUserAddressRequest, opts ...grpc.CallOption) (*userpb.CreateUserAddressResponse, error) {
	return &userpb.CreateUserAddressResponse{
		Address: &userpb.Address{
			Id:          "addr-1",
			UserId:      in.UserId,
			AddressLine: in.AddressLine,
			City:        in.City,
			State:       in.State,
			PostalCode:  in.PostalCode,
			Country:     in.Country,
			IsDefault:   in.IsDefault,
		},
	}, nil
}

func (m *mockUserClient) ListUserAddresses(ctx context.Context, in *userpb.ListUserAddressesRequest, opts ...grpc.CallOption) (*userpb.ListUserAddressesResponse, error) {
	return &userpb.ListUserAddressesResponse{
		Addresses: []*userpb.Address{
			{
				Id:          "addr-1",
				UserId:      in.UserId,
				AddressLine: "100 Main St",
				City:        "Austin",
				State:       "TX",
				PostalCode:  "78701",
				Country:     "United States",
				IsDefault:   true,
			},
		},
	}, nil
}

func (m *mockUserClient) GetUserAddress(ctx context.Context, in *userpb.GetUserAddressRequest, opts ...grpc.CallOption) (*userpb.GetUserAddressResponse, error) {
	return &userpb.GetUserAddressResponse{
		Address: &userpb.Address{
			Id:          in.AddressId,
			UserId:      in.UserId,
			AddressLine: "100 Main St",
			City:        "Austin",
			State:       "TX",
			PostalCode:  "78701",
			Country:     "United States",
			IsDefault:   true,
		},
	}, nil
}

func (m *mockUserClient) UpdateUserAddress(ctx context.Context, in *userpb.UpdateUserAddressRequest, opts ...grpc.CallOption) (*userpb.UpdateUserAddressResponse, error) {
	return &userpb.UpdateUserAddressResponse{
		Address: &userpb.Address{
			Id:          in.AddressId,
			UserId:      in.UserId,
			AddressLine: "Updated Line",
			City:        "Austin",
			State:       "TX",
			PostalCode:  "78701",
			Country:     "United States",
			IsDefault:   true,
		},
	}, nil
}

func (m *mockUserClient) DeleteUserAddress(ctx context.Context, in *userpb.DeleteUserAddressRequest, opts ...grpc.CallOption) (*userpb.DeleteUserAddressResponse, error) {
	return &userpb.DeleteUserAddressResponse{
		Success: true,
	}, nil
}

func (m *mockUserClient) SetDefaultUserAddress(ctx context.Context, in *userpb.SetDefaultUserAddressRequest, opts ...grpc.CallOption) (*userpb.SetDefaultUserAddressResponse, error) {
	return &userpb.SetDefaultUserAddressResponse{
		Address: &userpb.Address{
			Id:        in.AddressId,
			UserId:    in.UserId,
			IsDefault: true,
		},
	}, nil
}


func setupTestApp(introEnabled bool) *fiber.App {
	clients := gatewayGRPC.NewClientsWithServices(
		&mockUserClient{},
		&mockAuthClient{},
	)

	resolver := gwResolver.NewResolver(clients)
	cfg := &config.Config{
		GraphQLIntrospectionEnabled: introEnabled,
		JWT: config.JWTConfig{
			Secret: testJWTSecret,
			Issuer: "gocart-api-gateway",
		},
	}

	schema, _ := NewSchema(resolver, cfg)
	handler := NewHandler(schema, cfg)

	app := fiber.New()
	handler.RegisterRoutes(app)
	return app
}

func TestPublicQuery_HealthAndVersion(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `query { health version }`,
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
	if data["health"] != "OK" {
		t.Errorf("expected health 'OK', got %v", data["health"])
	}
}

func TestProtectedQuery_Me_Unauthenticated(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `query { me { id email } }`,
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
		t.Fatalf("expected authorization error for unauthenticated query, got %v", res)
	}

	firstErr := errors[0].(map[string]interface{})
	extensions, ok := firstErr["extensions"].(map[string]interface{})
	if !ok || extensions["code"] != "UNAUTHORIZED" {
		t.Errorf("expected UNAUTHORIZED error code, got extensions: %v", extensions)
	}
}

func TestProtectedQuery_Me_InvalidToken(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `query { me { id email } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer invalid-token-string")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	errors, ok := res["errors"].([]interface{})
	if !ok || len(errors) == 0 {
		t.Fatalf("expected error for invalid token, got %v", res)
	}

	firstErr := errors[0].(map[string]interface{})
	extensions := firstErr["extensions"].(map[string]interface{})
	if extensions["code"] != "UNAUTHORIZED" {
		t.Errorf("expected UNAUTHORIZED code, got %v", extensions["code"])
	}
}

func TestProtectedQuery_Me_ValidToken(t *testing.T) {
	app := setupTestApp(true)

	token, err := auth.GenerateToken(auth.UserContext{
		UserID: "u-100",
		Role:   "CUSTOMER",
		Email:  "user@example.com",
	}, testJWTSecret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	reqBody := map[string]interface{}{
		"query": `query { me { id email firstName } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	t.Logf("response body: %v", res)

	data, ok := res["data"].(map[string]interface{})
	if !ok || data == nil {
		t.Fatalf("expected data map, got %v", res)
	}
	me, ok := data["me"].(map[string]interface{})
	if !ok || me == nil {
		t.Fatalf("expected me map, got %v", data)
	}
	if me["id"] != "u-100" {
		t.Errorf("expected me.id 'u-100', got '%v'", me["id"])
	}
}

func TestRBAC_AdminRequired_CustomerRole(t *testing.T) {
	app := setupTestApp(true)

	token, _ := auth.GenerateToken(auth.UserContext{
		UserID: "u-100",
		Role:   "CUSTOMER",
	}, testJWTSecret, 1*time.Hour)

	reqBody := map[string]interface{}{
		"query": `query { user(id: "u-100") { id email } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	errors, ok := res["errors"].([]interface{})
	if !ok || len(errors) == 0 {
		t.Fatalf("expected error for customer role accessing admin query, got %v", res)
	}

	firstErr := errors[0].(map[string]interface{})
	extensions := firstErr["extensions"].(map[string]interface{})
	if extensions["code"] != "FORBIDDEN" {
		t.Errorf("expected FORBIDDEN code for insufficient role, got %v", extensions["code"])
	}
}

func TestRBAC_AdminRequired_AdminRole(t *testing.T) {
	app := setupTestApp(true)

	token, _ := auth.GenerateToken(auth.UserContext{
		UserID: "u-admin",
		Role:   "ADMIN",
	}, testJWTSecret, 1*time.Hour)

	reqBody := map[string]interface{}{
		"query": `query { user(id: "u-100") { id email } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data := res["data"].(map[string]interface{})
	user := data["user"].(map[string]interface{})
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
		"query": `mutation { register(input: {email: "new@gocart.com", password: "password123", firstName: "Jane", lastName: "Doe"}) { token user { id email } } }`,
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

	data := res["data"].(map[string]interface{})
	reg := data["register"].(map[string]interface{})

	if reg["token"] != "jwt-mock-register-token" {
		t.Errorf("expected token 'jwt-mock-register-token', got '%v'", reg["token"])
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
