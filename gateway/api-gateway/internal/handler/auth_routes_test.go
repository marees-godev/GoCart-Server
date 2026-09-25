package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	gatewayGRPC "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"google.golang.org/grpc"
)

type mockAuthClientForHTTP struct {
	authpb.AuthServiceClient
	verifyFunc func(ctx context.Context, in *authpb.VerifyEmailRequest, opts ...grpc.CallOption) (*authpb.VerifyEmailResponse, error)
}

func (m *mockAuthClientForHTTP) VerifyEmail(ctx context.Context, in *authpb.VerifyEmailRequest, opts ...grpc.CallOption) (*authpb.VerifyEmailResponse, error) {
	if m.verifyFunc != nil {
		return m.verifyFunc(ctx, in, opts...)
	}
	return &authpb.VerifyEmailResponse{Success: true, Message: "Verified"}, nil
}

func TestVerifyEmailHTTPRoute(t *testing.T) {
	app := fiber.New()
	mockAuth := &mockAuthClientForHTTP{}
	clients := &gatewayGRPC.Clients{
		AuthClient: mockAuth,
	}

	RegisterAuthRoutes(app, clients)

	t.Run("Missing token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/auth/verify-email", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != 400 {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("Valid token success", func(t *testing.T) {
		mockAuth.verifyFunc = func(ctx context.Context, in *authpb.VerifyEmailRequest, opts ...grpc.CallOption) (*authpb.VerifyEmailResponse, error) {
			if in.Token == "valid-token-123" {
				return &authpb.VerifyEmailResponse{Success: true, Message: "Verified"}, nil
			}
			return &authpb.VerifyEmailResponse{Success: false, Message: "Invalid"}, nil
		}

		req := httptest.NewRequest("GET", "/api/v1/auth/verify-email?token=valid-token-123", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("Invalid token failure", func(t *testing.T) {
		mockAuth.verifyFunc = func(ctx context.Context, in *authpb.VerifyEmailRequest, opts ...grpc.CallOption) (*authpb.VerifyEmailResponse, error) {
			return &authpb.VerifyEmailResponse{Success: false, Message: "Token expired"}, nil
		}

		req := httptest.NewRequest("GET", "/api/v1/auth/verify-email?token=invalid-token", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != 400 {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})
}
