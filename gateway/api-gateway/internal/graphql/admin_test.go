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
	gatewayGRPC "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	testAdminKey    = "test-admin-secret-key-123"
	testAdminUserID = "admin-system-id"
)

func setupAdminTestApp() *fiber.App {
	clients := gatewayGRPC.NewClientsWithServices(
		&mockUserClient{},
	)

	resolver := gwResolver.NewResolver(clients)
	cfg := &config.Config{
		GraphQLIntrospectionEnabled: true,
		AdminAPIKey:                 testAdminKey,
		AdminUserID:                 testAdminUserID,
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

func TestAdmin_ValidAPIKey_UnrestrictedAccess(t *testing.T) {
	app := setupAdminTestApp()

	// Perform a query requiring ADMIN role (@auth(requires: [ADMIN]))
	reqBody := map[string]interface{}{
		"query": `query { user(id: "u-100") { id email } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderXAdminKey, testAdminKey)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	if res["errors"] != nil {
		t.Fatalf("expected no errors for valid Admin API Key, got %v", res["errors"])
	}

	data, ok := res["data"].(map[string]interface{})
	if !ok || data["user"] == nil {
		t.Fatalf("expected user data response, got %v", res)
	}
}

func TestAdmin_InvalidAPIKey_ReturnsUnauthorized(t *testing.T) {
	app := setupAdminTestApp()

	reqBody := map[string]interface{}{
		"query": `query { health }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderXAdminKey, "wrong-admin-key")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized for invalid API key, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	errors, ok := res["errors"].([]interface{})
	if !ok || len(errors) == 0 {
		t.Fatalf("expected errors slice in response, got %v", res)
	}

	firstErr := errors[0].(map[string]interface{})
	extensions, _ := firstErr["extensions"].(map[string]interface{})
	if extensions["code"] != "UNAUTHORIZED" {
		t.Errorf("expected UNAUTHORIZED error code, got %v", extensions["code"])
	}
}

func TestAdmin_MissingAPIKey_FallsBackToJWT(t *testing.T) {
	app := setupAdminTestApp()

	// 1. Without Admin Key and without JWT -> fails on protected resource
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
	if res["errors"] == nil {
		t.Fatalf("expected authorization error without auth headers, got %v", res)
	}

	// 2. Without Admin Key but with valid JWT -> succeeds via JWT
	token, _ := auth.GenerateToken(auth.UserContext{
		UserID: "jwt-user-123",
		Role:   "CUSTOMER",
	}, testJWTSecret, 1*time.Hour)

	reqJWT := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	reqJWT.Header.Set("Content-Type", "application/json")
	reqJWT.Header.Set("Authorization", "Bearer "+token)

	respJWT, err := app.Test(reqJWT)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if respJWT.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 with valid JWT fallback, got %d", respJWT.StatusCode)
	}
}

func TestAdmin_ClientIdentityHeaderOverridePrevention(t *testing.T) {
	app := setupAdminTestApp()

	reqBody := map[string]interface{}{
		"query": `query { user(id: "u-100") { id email } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderXAdminKey, testAdminKey)
	// Spoofed identity headers in incoming HTTP request
	req.Header.Set("x-user-id", "attacker-id")
	req.Header.Set("x-user-role", "CUSTOMER")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	if res["errors"] != nil {
		t.Fatalf("expected no errors for valid Admin request despite spoofed headers, got %v", res["errors"])
	}
}

func TestAdmin_gRPCMetadataPropagation(t *testing.T) {
	ctx := context.Background()
	userCtx := &auth.UserContext{
		UserID: testAdminUserID,
		Role:   "ADMIN",
	}
	ctx = auth.WithUser(ctx, userCtx)

	// Create interceptor
	interceptor := grpcclient.UnaryClientInterceptor(5 * time.Second)

	var capturedMD metadata.MD
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		if md, ok := metadata.FromOutgoingContext(ctx); ok {
			capturedMD = md
		}
		return nil
	}

	err := interceptor(ctx, "/userpb.UserService/GetUser", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("unexpected interceptor error: %v", err)
	}

	userIDs := capturedMD.Get(grpcclient.HeaderUserID)
	if len(userIDs) == 0 || userIDs[0] != testAdminUserID {
		t.Errorf("expected x-user-id '%s', got %v", testAdminUserID, userIDs)
	}

	userRoles := capturedMD.Get(grpcclient.HeaderUserRole)
	if len(userRoles) == 0 || userRoles[0] != "ADMIN" {
		t.Errorf("expected x-user-role 'ADMIN', got %v", userRoles)
	}

	reqIDs := capturedMD.Get(grpcclient.HeaderRequestID)
	if len(reqIDs) == 0 || reqIDs[0] == "" {
		t.Errorf("expected non-empty x-request-id in metadata, got %v", reqIDs)
	}
}
