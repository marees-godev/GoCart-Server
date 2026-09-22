package tests

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gofrs/uuid/v5"
	pb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
	authGRPC "github.com/marees-godev/GoCart-Server/services/auth-service/internal/handler/grpc"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/service"
	"golang.org/x/crypto/bcrypt"
)

type mockRepoForGRPC struct {
	user *model.AuthCredential
}

func (m *mockRepoForGRPC) GetByEmail(ctx context.Context, email string) (*model.AuthCredential, error) {
	if m.user != nil && m.user.Email == email {
		return m.user, nil
	}
	return nil, repository.ErrNotFound
}

func (m *mockRepoForGRPC) GetByEmailAndRole(ctx context.Context, email, role string) (*model.AuthCredential, error) {
	if m.user != nil && m.user.Email == email && m.user.Role == role {
		return m.user, nil
	}
	return nil, repository.ErrNotFound
}

func (m *mockRepoForGRPC) UpdateFailedLogin(ctx context.Context, id uuid.UUID, failedCount int, lockedUntil *time.Time) error {
	return nil
}

func (m *mockRepoForGRPC) ResetFailedLogin(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockRepoForGRPC) CreateLoginSession(ctx context.Context, refreshToken *model.RefreshToken, evt *outbox.Event) error {
	return nil
}

func (m *mockRepoForGRPC) CreateCredential(ctx context.Context, cred *model.AuthCredential) error {
	return nil
}

func (m *mockRepoForGRPC) GetRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	return nil, repository.ErrNotFound
}

func (m *mockRepoForGRPC) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	return nil
}

func TestGRPCLogin_SuccessAndClaims(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	rawPassword := "Password123!"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	user := &model.AuthCredential{
		ID:           uuid.Must(uuid.NewV7()),
		UserID:       userID,
		Email:        "user@example.com",
		PasswordHash: string(hashed),
		Role:         "customer",
		IsActive:     true,
	}

	repo := &mockRepoForGRPC{user: user}
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "integration-jwt-secret-999",
			ExpiryMinutes: 15,
		},
	}
	svc := service.NewAuthService(repo, cfg, nil)
	grpcHandler := authGRPC.NewAuthGRPCHandler(svc, nil)

	loginResp, err := grpcHandler.Login(context.Background(), &pb.LoginRequest{
		Email:    "user@example.com",
		Password: rawPassword,
	})
	if err != nil {
		t.Fatalf("expected successful gRPC login, got: %v", err)
	}

	if loginResp.AccessToken == "" {
		t.Error("expected non-empty access_token")
	}
	if loginResp.RefreshToken == "" {
		t.Error("expected non-empty refresh_token")
	}
	if loginResp.TokenType != "Bearer" {
		t.Errorf("expected token_type Bearer, got %s", loginResp.TokenType)
	}
	if loginResp.ExpiresIn != 900 {
		t.Errorf("expected expires_in 900, got %d", loginResp.ExpiresIn)
	}

	// Parse JWT claims
	claims := &auth.UserClaims{}
	token, err := jwt.ParseWithClaims(loginResp.AccessToken, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(cfg.JWT.Secret), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("access token invalid: %v", err)
	}

	if claims.Subject != userID.String() {
		t.Errorf("expected sub claim %s, got %s", userID.String(), claims.Subject)
	}
	if claims.Role != "customer" {
		t.Errorf("expected role claim customer, got %s", claims.Role)
	}
	if claims.ID == "" {
		t.Error("expected jti claim to be present")
	}
}

func TestGRPCLogin_InvalidCredentials(t *testing.T) {
	repo := &mockRepoForGRPC{user: nil}
	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "secret"},
	}
	svc := service.NewAuthService(repo, cfg, nil)
	grpcHandler := authGRPC.NewAuthGRPCHandler(svc, nil)

	_, err := grpcHandler.Login(context.Background(), &pb.LoginRequest{
		Email:    "nonexistent@example.com",
		Password: "Password123!",
	})
	if err == nil {
		t.Fatal("expected error for invalid credentials, got nil")
	}
}
