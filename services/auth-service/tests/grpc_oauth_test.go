package tests

import (
	"context"
	"testing"

	pb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
	authGRPC "github.com/marees-godev/GoCart-Server/services/auth-service/internal/handler/grpc"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/service"
)

type mockFacebookVerifierForGRPC struct {
	payload *service.FacebookTokenPayload
	err     error
}

func (m *mockFacebookVerifierForGRPC) Verify(ctx context.Context, token string) (*service.FacebookTokenPayload, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.payload, nil
}

func (m *mockFacebookVerifierForGRPC) ExchangeCode(ctx context.Context, code, redirectURI string) (string, error) {
	return "mock-fb-token", nil
}

func TestGRPC_FacebookOAuth(t *testing.T) {
	repo := newInMemoryAuthRepo()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-grpc",
			ExpiryMinutes: 15,
		},
	}
	fbVerifier := &mockFacebookVerifierForGRPC{
		payload: &service.FacebookTokenPayload{
			UserID:        "fb-grpc-123",
			Email:         "fbuser@example.com",
			EmailVerified: true,
			FirstName:     "Frank",
			LastName:      "Book",
		},
	}

	authSvc := service.NewAuthService(repo, cfg, nil, &mockUserServiceClient{}, fbVerifier)
	handler := authGRPC.NewAuthGRPCHandler(authSvc, nil)

	ctx := context.Background()

	// 1. Register with Facebook via gRPC
	regResp, err := handler.FacebookRegister(ctx, &pb.FacebookAuthRequest{
		AccessToken: "valid-fb-token",
		Role:        pb.Role_USER,
	})
	if err != nil {
		t.Fatalf("unexpected error registering with facebook via grpc: %v", err)
	}
	if regResp.AccessToken == "" {
		t.Errorf("expected non-empty access token")
	}
	if regResp.UserId == "" {
		t.Errorf("expected non-empty user_id")
	}

	// 2. Login with Facebook via gRPC
	loginResp, err := handler.FacebookLogin(ctx, &pb.FacebookAuthRequest{
		AccessToken: "valid-fb-token",
		Role:        pb.Role_USER,
	})
	if err != nil {
		t.Fatalf("unexpected error logging in with facebook via grpc: %v", err)
	}
	if loginResp.UserId != regResp.UserId {
		t.Errorf("expected user_id %s, got %s", regResp.UserId, loginResp.UserId)
	}
}

