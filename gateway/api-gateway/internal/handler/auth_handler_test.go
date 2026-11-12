package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	grpcPkg "google.golang.org/grpc"
)

type mockAuthGRPCClient struct {
	authpb.AuthServiceClient
	lastGoogleReq *authpb.GoogleAuthRequest
	resp          *authpb.AuthResponse
	err           error
}

func (m *mockAuthGRPCClient) GoogleAuth(ctx context.Context, in *authpb.GoogleAuthRequest, opts ...grpcPkg.CallOption) (*authpb.AuthResponse, error) {
	m.lastGoogleReq = in
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func (m *mockAuthGRPCClient) GoogleLogin(ctx context.Context, in *authpb.GoogleAuthRequest, opts ...grpcPkg.CallOption) (*authpb.AuthResponse, error) {
	return m.GoogleAuth(ctx, in, opts...)
}

func (m *mockAuthGRPCClient) GoogleRegister(ctx context.Context, in *authpb.GoogleAuthRequest, opts ...grpcPkg.CallOption) (*authpb.AuthResponse, error) {
	return m.GoogleAuth(ctx, in, opts...)
}

type mockTransport func(req *http.Request) (*http.Response, error)

func (m mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func TestHandleGoogleLogin(t *testing.T) {
	t.Run("missing client ID returns 400", func(t *testing.T) {
		app := fiber.New()
		cfg := &config.Config{
			Google: config.GoogleConfig{
				ClientID: "",
			},
		}
		h := NewAuthHandler(cfg, &mockAuthGRPCClient{}, nil)
		h.RegisterRoutes(app)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("valid client ID redirects to Google", func(t *testing.T) {
		app := fiber.New()
		cfg := &config.Config{
			Google: config.GoogleConfig{
				ClientID:    "test-client-id.apps.googleusercontent.com",
				RedirectURL: "http://localhost:8080/api/v1/auth/google/callback",
			},
		}
		h := NewAuthHandler(cfg, &mockAuthGRPCClient{}, nil)
		h.RegisterRoutes(app)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google?role=USER", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusTemporaryRedirect {
			t.Errorf("expected 307 redirect, got %d", resp.StatusCode)
		}

		location := resp.Header.Get("Location")
		if location == "" {
			t.Fatal("expected Location header in redirect")
		}
		if !bytes.Contains([]byte(location), []byte("accounts.google.com")) {
			t.Errorf("expected redirect to accounts.google.com, got: %s", location)
		}
		if !bytes.Contains([]byte(location), []byte("test-client-id")) {
			t.Errorf("expected client_id in location, got: %s", location)
		}
	})
}

func TestHandleGoogleCallback(t *testing.T) {
	t.Run("google error returns 400", func(t *testing.T) {
		app := fiber.New()
		cfg := &config.Config{}
		h := NewAuthHandler(cfg, &mockAuthGRPCClient{}, nil)
		h.RegisterRoutes(app)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?error=access_denied", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("missing code returns 400", func(t *testing.T) {
		app := fiber.New()
		cfg := &config.Config{}
		h := NewAuthHandler(cfg, &mockAuthGRPCClient{}, nil)
		h.RegisterRoutes(app)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("code exchange success", func(t *testing.T) {
		app := fiber.New()
		cfg := &config.Config{
			Google: config.GoogleConfig{
				ClientID:     "google-client-id",
				ClientSecret: "google-client-secret",
				RedirectURL:  "http://localhost:8080/api/v1/auth/google/callback",
			},
		}
		mockClient := &mockAuthGRPCClient{
			resp: &authpb.AuthResponse{
				AccessToken:  "gocart-access-token",
				RefreshToken: "gocart-refresh-token",
				TokenType:    "Bearer",
				ExpiresIn:    900,
				UserId:       "user-123",
				Role:         authpb.Role_USER,
			},
		}
		h := NewAuthHandler(cfg, mockClient, nil)
		h.httpClient = &http.Client{
			Transport: mockTransport(func(req *http.Request) (*http.Response, error) {
				body, _ := json.Marshal(map[string]any{
					"access_token": "google-access-token",
					"id_token":     "exchanged-id-token-xyz",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			}),
		}
		h.RegisterRoutes(app)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=google-auth-code-123", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		if mockClient.lastGoogleReq == nil || mockClient.lastGoogleReq.IdToken != "exchanged-id-token-xyz" {
			t.Errorf("expected id_token 'exchanged-id-token-xyz', got %v", mockClient.lastGoogleReq)
		}
	})

	t.Run("register callback invokes GoogleRegister", func(t *testing.T) {
		app := fiber.New()
		cfg := &config.Config{
			Google: config.GoogleConfig{
				ClientID:     "google-client-id",
				ClientSecret: "google-client-secret",
				RedirectURL:  "http://localhost:8080/api/v1/auth/google/callback",
			},
		}
		mockClient := &mockAuthGRPCClient{
			resp: &authpb.AuthResponse{
				AccessToken:  "gocart-reg-token",
				RefreshToken: "gocart-reg-refresh-token",
				TokenType:    "Bearer",
				ExpiresIn:    900,
				UserId:       "user-reg-123",
				Role:         authpb.Role_USER,
			},
		}
		h := NewAuthHandler(cfg, mockClient, nil)
		h.httpClient = &http.Client{
			Transport: mockTransport(func(req *http.Request) (*http.Response, error) {
				body, _ := json.Marshal(map[string]any{
					"access_token": "google-access-token",
					"id_token":     "reg-id-token-xyz",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			}),
		}
		h.RegisterRoutes(app)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=code-123&state=USER:register", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		if mockClient.lastGoogleReq == nil || mockClient.lastGoogleReq.IdToken != "reg-id-token-xyz" {
			t.Errorf("expected id_token 'reg-id-token-xyz', got %v", mockClient.lastGoogleReq)
		}
	})
}

