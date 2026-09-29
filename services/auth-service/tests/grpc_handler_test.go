package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/golang-jwt/jwt/v5"
	pb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
	authGRPC "github.com/marees-godev/GoCart-Server/services/auth-service/internal/handler/grpc"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/service"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockUserServiceClient struct {
	userpb.UserServiceClient
}

func (m *mockUserServiceClient) CreateUser(ctx context.Context, req *userpb.CreateUserRequest, opts ...grpc.CallOption) (*userpb.CreateUserResponse, error) {
	return &userpb.CreateUserResponse{
		User: &userpb.User{
			Id:        req.Id,
			Email:     req.Email,
			FirstName: req.FirstName,
			LastName:  req.LastName,
		},
	}, nil
}

type mockMerchantClient struct {
	merchantpb.MerchantServiceClient
	createdMerchants []*merchantpb.CreateMerchantRequest
}

func (m *mockMerchantClient) CreateMerchant(ctx context.Context, in *merchantpb.CreateMerchantRequest, opts ...grpc.CallOption) (*merchantpb.CreateMerchantResponse, error) {
	m.createdMerchants = append(m.createdMerchants, in)
	return &merchantpb.CreateMerchantResponse{
		Merchant: &merchantpb.Merchant{
			Id:            in.UserId,
			UserId:        in.UserId,
			BusinessName:  in.BusinessName,
			BusinessEmail: in.BusinessEmail,
			FirstName:     in.FirstName,
			LastName:      in.LastName,
		},
	}, nil
}

type inMemoryAuthRepo struct {
	byEmailRole  map[string]*model.AuthCredential
	outboxEvents []*outbox.Event
}

func newInMemoryAuthRepo() *inMemoryAuthRepo {
	return &inMemoryAuthRepo{
		byEmailRole:  make(map[string]*model.AuthCredential),
		outboxEvents: make([]*outbox.Event, 0),
	}
}

func (r *inMemoryAuthRepo) GetByEmail(ctx context.Context, email string) (*model.AuthCredential, error) {
	for _, cred := range r.byEmailRole {
		if strings.EqualFold(cred.Email, email) {
			return cred, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (r *inMemoryAuthRepo) GetByEmailAndRole(ctx context.Context, email string, role model.Role) (*model.AuthCredential, error) {
	key := strings.ToLower(email) + ":" + strings.ToUpper(role.String())
	cred, ok := r.byEmailRole[key]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return cred, nil
}

func (r *inMemoryAuthRepo) CreateCredential(ctx context.Context, cred *model.AuthCredential) error {
	key := strings.ToLower(cred.Email) + ":" + strings.ToUpper(cred.Role.String())
	if _, exists := r.byEmailRole[key]; exists {
		return appErrors.Conflict("user with this email and role already exists")
	}
	r.byEmailRole[key] = cred
	return nil
}

func (r *inMemoryAuthRepo) DeleteCredential(ctx context.Context, id uuid.UUID) error {
	for k, cred := range r.byEmailRole {
		if cred.ID == id {
			delete(r.byEmailRole, k)
			return nil
		}
	}
	return nil
}

func (r *inMemoryAuthRepo) UpdateFailedLogin(ctx context.Context, id uuid.UUID, failedCount int, lockedUntil *time.Time) error {
	return nil
}

func (r *inMemoryAuthRepo) ResetFailedLogin(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (r *inMemoryAuthRepo) CreateLoginSession(ctx context.Context, refreshToken *model.RefreshToken, evt *outbox.Event) error {
	if evt != nil {
		r.outboxEvents = append(r.outboxEvents, evt)
	}
	return nil
}

func (r *inMemoryAuthRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*model.AuthCredential, error) {
	for _, cred := range r.byEmailRole {
		if cred.UserID == userID {
			return cred, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (r *inMemoryAuthRepo) GetRefreshToken(ctx context.Context, tokenHash string, userIDOrEmail string) (*model.RefreshToken, error) {
	return nil, repository.ErrNotFound
}

func (r *inMemoryAuthRepo) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (r *inMemoryAuthRepo) RevokeRefreshTokenByHash(ctx context.Context, tokenHash string) error {
	return nil
}

func (r *inMemoryAuthRepo) RevokeRefreshTokensByUserID(ctx context.Context, userID uuid.UUID) error {
	return nil
}

func (r *inMemoryAuthRepo) RotateRefreshToken(ctx context.Context, oldTokenID uuid.UUID, newToken *model.RefreshToken) error {
	return nil
}

func (r *inMemoryAuthRepo) MarkEmailVerified(ctx context.Context, userID uuid.UUID) error {
	return nil
}

func (r *inMemoryAuthRepo) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string, outboxEvent *outbox.Event) error {
	for _, cred := range r.byEmailRole {
		if cred.UserID == userID {
			cred.PasswordHash = passwordHash
			if outboxEvent != nil {
				r.outboxEvents = append(r.outboxEvents, outboxEvent)
			}
			return nil
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Registration Tests
// ---------------------------------------------------------------------------

func TestGRPC_MultiRoleRegistrationAndUniqueness(t *testing.T) {
	repo := newInMemoryAuthRepo()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key",
			ExpiryMinutes: 15,
		},
	}
	mockMerchant := &mockMerchantClient{}
	authSvc := service.NewAuthService(repo, cfg, nil, &mockUserServiceClient{}, mockMerchant)
	handler := authGRPC.NewAuthGRPCHandler(authSvc, nil)

	ctx := context.Background()
	testEmail := "john@example.com"
	testPassword := "SecurePassword123!"

	// 1. Register john@example.com as a CUSTOMER -> Success
	custResp, err := handler.Register(ctx, &pb.RegisterRequest{
		Email:      testEmail,
		Password:   testPassword,
		FirstName:  "John",
		LastName:   "Customer",
		IsMerchant: false,
	})
	if err != nil {
		t.Fatalf("expected customer registration to succeed, got error: %v", err)
	}
	if custResp.AccessToken == "" || custResp.UserId == "" {
		t.Fatalf("expected valid token and userId for customer, got %+v", custResp)
	}

	custCred, _ := repo.GetByEmailAndRole(ctx, testEmail, model.RoleCustomer)
	if custCred == nil || custCred.Role != model.RoleCustomer {
		t.Fatalf("expected CUSTOMER credential to be saved in repository")
	}

	// 2. Register john@example.com as a MERCHANT -> Success
	merchResp, err := handler.Register(ctx, &pb.RegisterRequest{
		Email:      testEmail,
		Password:   testPassword,
		FirstName:  "John",
		LastName:   "Merchant",
		IsMerchant: true,
	})
	if err != nil {
		t.Fatalf("expected merchant registration with same email to succeed, got error: %v", err)
	}
	if merchResp.AccessToken == "" || merchResp.UserId == "" {
		t.Fatalf("expected valid token and userId for merchant, got %+v", merchResp)
	}
	if custResp.UserId == merchResp.UserId {
		t.Errorf("expected distinct UserIds for customer and merchant, got identical: %s", custResp.UserId)
	}

	merchCred, _ := repo.GetByEmailAndRole(ctx, testEmail, model.RoleMerchant)
	if merchCred == nil || merchCred.Role != model.RoleMerchant {
		t.Fatalf("expected MERCHANT credential to be saved in repository")
	}

	if len(mockMerchant.createdMerchants) != 1 {
		t.Fatalf("expected 1 call to merchant service CreateMerchant, got %d", len(mockMerchant.createdMerchants))
	}
	if mockMerchant.createdMerchants[0].UserId != merchResp.UserId {
		t.Errorf("expected merchant ID %s, got %s", merchResp.UserId, mockMerchant.createdMerchants[0].UserId)
	}
	if mockMerchant.createdMerchants[0].BusinessName != "John Merchant" {
		t.Errorf("expected business name John Merchant, got %s", mockMerchant.createdMerchants[0].BusinessName)
	}

	// 3. Registering john@example.com again as a CUSTOMER -> Fails with 409 Conflict
	_, err = handler.Register(ctx, &pb.RegisterRequest{
		Email:      testEmail,
		Password:   testPassword,
		FirstName:  "Duplicate",
		LastName:   "Customer",
		IsMerchant: false,
	})
	if err == nil {
		t.Fatalf("expected conflict error when re-registering as customer, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.HTTPStatus != 409 {
		t.Errorf("expected HTTP 409 Conflict, got %d (err: %v)", appErr.HTTPStatus, err)
	}

	// 4. Registering john@example.com again as a MERCHANT -> Fails with 409 Conflict
	_, err = handler.Register(ctx, &pb.RegisterRequest{
		Email:      testEmail,
		Password:   testPassword,
		FirstName:  "Duplicate",
		LastName:   "Merchant",
		IsMerchant: true,
	})
	if err == nil {
		t.Fatalf("expected conflict error when re-registering as merchant, got nil")
	}
	appErr = appErrors.AsAppError(err)
	if appErr.HTTPStatus != 409 {
		t.Errorf("expected HTTP 409 Conflict, got %d (err: %v)", appErr.HTTPStatus, err)
	}

	// 5. Verify outbox events: exactly 2 events created (1 customer, 1 merchant)
	if len(repo.outboxEvents) != 2 {
		t.Fatalf("expected 2 outbox events, got %d", len(repo.outboxEvents))
	}
	if repo.outboxEvents[0].Topic != "auth.user.registered" || repo.outboxEvents[0].EventType != "UserRegistered" {
		t.Errorf("expected first event to be auth.user.registered, got %s / %s", repo.outboxEvents[0].Topic, repo.outboxEvents[0].EventType)
	}
	if repo.outboxEvents[1].Topic != "auth.merchant.registered" || repo.outboxEvents[1].EventType != "MerchantRegistered" {
		t.Errorf("expected second event to be auth.merchant.registered, got %s / %s", repo.outboxEvents[1].Topic, repo.outboxEvents[1].EventType)
	}
}

// ---------------------------------------------------------------------------
// Login Tests
// ---------------------------------------------------------------------------

func TestGRPCLogin_SuccessAndClaims(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	rawPassword := "Password123!"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	user := &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        userID,
		Email:         "user@example.com",
		PasswordHash:  string(hashed),
		Role:          model.RoleCustomer,
		EmailVerified: true,
		IsActive:      true,
	}

	repo := newInMemoryAuthRepo()
	_ = repo.CreateCredential(context.Background(), user)

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
	repo := newInMemoryAuthRepo()
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
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        userID,
		Email:         "merchant@example.com",
		PasswordHash:  string(hashed),
		Role:          model.RoleMerchant,
		EmailVerified: true,
		IsActive:      true,
	}

	repo := newInMemoryAuthRepo()
	_ = repo.CreateCredential(context.Background(), user)

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

	repo := newInMemoryAuthRepo()
	_ = repo.CreateCredential(context.Background(), user)

	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "secret"},
	}
	svc := service.NewAuthService(repo, cfg, nil)
	grpcHandler := authGRPC.NewAuthGRPCHandler(svc, nil)

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

	repo := newInMemoryAuthRepo()
	_ = repo.CreateCredential(context.Background(), user)

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
