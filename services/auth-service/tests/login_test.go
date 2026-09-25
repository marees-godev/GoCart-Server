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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func (m *mockRepoForGRPC) GetByEmailAndRole(ctx context.Context, email string, role model.Role) (*model.AuthCredential, error) {
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

func (m *mockRepoForGRPC) DeleteCredential(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockRepoForGRPC) GetRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	return nil, repository.ErrNotFound
}

func (m *mockRepoForGRPC) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockRepoForGRPC) MarkEmailVerified(ctx context.Context, userID uuid.UUID) error {
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
		Role:         model.RoleCustomer,
		EmailVerified: true,
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
	if claims.Role != model.RoleCustomer.String() {
		t.Errorf("expected role claim %s, got %s", model.RoleCustomer, claims.Role)
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

func TestGRPCLogin_MerchantSuccess(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	rawPassword := "Password123!"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	user := &model.AuthCredential{
		ID:           uuid.Must(uuid.NewV7()),
		UserID:       userID,
		Email:        "merchant@example.com",
		PasswordHash: string(hashed),
		Role:         model.RoleMerchant,
		EmailVerified: true,
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
		Email:      "merchant@example.com",
		Password:   rawPassword,
		IsMerchant: true,
	})
	if err != nil {
		t.Fatalf("expected successful merchant gRPC login, got: %v", err)
	}

	if loginResp.Role != "MERCHANT" {
		t.Errorf("expected role MERCHANT, got %s", loginResp.Role)
	}

	claims := &auth.UserClaims{}
	token, err := jwt.ParseWithClaims(loginResp.AccessToken, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(cfg.JWT.Secret), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("access token invalid: %v", err)
	}
	if claims.Role != "MERCHANT" {
		t.Errorf("expected claim role MERCHANT, got %s", claims.Role)
	}
}

func TestGRPCLogin_RoleMismatch(t *testing.T) {
	rawPassword := "Password123!"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	user := &model.AuthCredential{
		ID:           uuid.Must(uuid.NewV7()),
		UserID:       uuid.Must(uuid.NewV7()),
		Email:        "merchant@example.com",
		PasswordHash: string(hashed),
		Role:         model.RoleMerchant,
		IsActive:     true,
	}

	repo := &mockRepoForGRPC{user: user}
	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "secret"},
	}
	svc := service.NewAuthService(repo, cfg, nil)
	grpcHandler := authGRPC.NewAuthGRPCHandler(svc, nil)

	// Attempt to log into merchant account with IsMerchant: false (customer)
	_, err := grpcHandler.Login(context.Background(), &pb.LoginRequest{
		Email:      "merchant@example.com",
		Password:   rawPassword,
		IsMerchant: false,
	})
	if err == nil {
		t.Fatal("expected error due to role mismatch, got nil")
	}
}

func TestGRPCLogin_UnverifiedEmail_Fails(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	rawPassword := "Password123!"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	user := &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        userID,
		Email:         "unverified@example.com",
		PasswordHash:  string(hashed),
		Role:          model.RoleCustomer,
		EmailVerified: false,
		IsActive:      true,
	}

	repo := &mockRepoForGRPC{user: user}
	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "secret"},
	}
	svc := service.NewAuthService(repo, cfg, nil)
	grpcHandler := authGRPC.NewAuthGRPCHandler(svc, nil)

	_, err := grpcHandler.Login(context.Background(), &pb.LoginRequest{
		Email:      "unverified@example.com",
		Password:   rawPassword,
		IsMerchant: false,
	})
	if err == nil {
		t.Fatal("expected error for unverified email, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected codes.PermissionDenied, got %v", st.Code())
	}
	expectedMsg := "email is not verified, please verify your email first and then login"
	if st.Message() != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, st.Message())
	}
}
