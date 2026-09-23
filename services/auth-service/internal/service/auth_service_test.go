package service

import (
	"context"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/golang-jwt/jwt/v5"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/repository"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
)

type mockAuthRepository struct {
	byEmail           map[string]*model.AuthCredential
	byEmailRole       map[string]*model.AuthCredential
	failedCountMap    map[uuid.UUID]int
	lockedUntilMap    map[uuid.UUID]*time.Time
	refreshTokens     []*model.RefreshToken
	outboxEvents      []*outbox.Event
	getByEmailErr     error
	createSessionErr  error
}

func newMockAuthRepository() *mockAuthRepository {
	return &mockAuthRepository{
		byEmail:        make(map[string]*model.AuthCredential),
		byEmailRole:    make(map[string]*model.AuthCredential),
		failedCountMap: make(map[uuid.UUID]int),
		lockedUntilMap: make(map[uuid.UUID]*time.Time),
	}
}

func (m *mockAuthRepository) GetByEmail(ctx context.Context, email string) (*model.AuthCredential, error) {
	if m.getByEmailErr != nil {
		return nil, m.getByEmailErr
	}
	cred, ok := m.byEmail[email]
	if !ok {
		return nil, repository.ErrNotFound
	}
	c := *cred
	if fc, ok := m.failedCountMap[cred.ID]; ok {
		c.FailedLoginCount = fc
	}
	if lu, ok := m.lockedUntilMap[cred.ID]; ok {
		c.LockedUntil = lu
	}
	return &c, nil
}

func (m *mockAuthRepository) GetByEmailAndRole(ctx context.Context, email string, role model.Role) (*model.AuthCredential, error) {
	if m.getByEmailErr != nil {
		return nil, m.getByEmailErr
	}
	key := email + ":" + role.String()
	cred, ok := m.byEmailRole[key]
	if !ok {
		return nil, repository.ErrNotFound
	}
	c := *cred
	return &c, nil
}

func (m *mockAuthRepository) UpdateFailedLogin(ctx context.Context, id uuid.UUID, failedCount int, lockedUntil *time.Time) error {
	m.failedCountMap[id] = failedCount
	m.lockedUntilMap[id] = lockedUntil
	return nil
}

func (m *mockAuthRepository) ResetFailedLogin(ctx context.Context, id uuid.UUID) error {
	m.failedCountMap[id] = 0
	m.lockedUntilMap[id] = nil
	return nil
}

func (m *mockAuthRepository) CreateLoginSession(ctx context.Context, refreshToken *model.RefreshToken, evt *outbox.Event) error {
	if m.createSessionErr != nil {
		return m.createSessionErr
	}
	m.refreshTokens = append(m.refreshTokens, refreshToken)
	if evt != nil {
		m.outboxEvents = append(m.outboxEvents, evt)
	}
	return nil
}

func (m *mockAuthRepository) CreateCredential(ctx context.Context, cred *model.AuthCredential) error {
	if m.byEmail == nil {
		m.byEmail = make(map[string]*model.AuthCredential)
	}
	if m.byEmailRole == nil {
		m.byEmailRole = make(map[string]*model.AuthCredential)
	}
	m.byEmail[cred.Email] = cred
	m.byEmailRole[cred.Email+":"+cred.Role.String()] = cred
	return nil
}

func (m *mockAuthRepository) DeleteCredential(ctx context.Context, id uuid.UUID) error {
	for k, v := range m.byEmail {
		if v.ID == id {
			delete(m.byEmail, k)
			break
		}
	}
	return nil
}

func (m *mockAuthRepository) GetRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	for _, rt := range m.refreshTokens {
		if rt.TokenHash == tokenHash {
			return rt, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *mockAuthRepository) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	for _, rt := range m.refreshTokens {
		if rt.ID == id {
			rt.Revoked = true
			return nil
		}
	}
	return nil
}

func setupTestService() (AuthService, *mockAuthRepository, *config.Config) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-12345",
			ExpiryMinutes: 15,
		},
	}
	svc := NewAuthService(mockRepo, cfg, nil, &mockUserServiceClient{})
	return svc, mockRepo, cfg
}

func TestLogin_Success(t *testing.T) {
	svc, mockRepo, cfg := setupTestService()

	userID := uuid.Must(uuid.NewV7())
	credID := uuid.Must(uuid.NewV7())
	rawPassword := "StrongPassword123!"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	mockRepo.byEmail["user@example.com"] = &model.AuthCredential{
		ID:               credID,
		UserID:           userID,
		Email:            "user@example.com",
		PasswordHash:     string(hashedPassword),
		Role:             model.RoleCustomer,
		IsActive:         true,
		FailedLoginCount: 0,
	}

	req := &dto.LoginRequest{
		Email:    "user@example.com",
		Password: rawPassword,
	}

	resp, err := svc.Login(context.Background(), req)
	if err != nil {
		t.Fatalf("expected successful login, got err: %v", err)
	}

	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if resp.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("expected token_type Bearer, got %s", resp.TokenType)
	}
	if resp.ExpiresIn != 15*60 {
		t.Errorf("expected expires_in 900 seconds, got %d", resp.ExpiresIn)
	}

	// Verify JWT claims
	claims := &auth.UserClaims{}
	token, err := jwt.ParseWithClaims(resp.AccessToken, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(cfg.JWT.Secret), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("failed to parse generated access token: %v", err)
	}

	if claims.Subject != userID.String() {
		t.Errorf("expected sub %s, got %s", userID.String(), claims.Subject)
	}
	if claims.Role != model.RoleCustomer.String() {
		t.Errorf("expected role %s, got %s", model.RoleCustomer, claims.Role)
	}
	if claims.ID == "" {
		t.Error("expected jti claim to be populated")
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil {
		t.Error("expected iat and exp claims to be populated")
	}

	// Verify refresh token created in repo
	if len(mockRepo.refreshTokens) != 1 {
		t.Fatalf("expected 1 refresh token recorded, got %d", len(mockRepo.refreshTokens))
	}
	rt := mockRepo.refreshTokens[0]
	if rt.UserID != userID {
		t.Errorf("expected refresh token user_id %s, got %s", userID, rt.UserID)
	}
	if rt.TokenHash == "" {
		t.Error("expected refresh token hash to be non-empty")
	}

	// Verify UserLoggedIn outbox event
	if len(mockRepo.outboxEvents) != 1 {
		t.Fatalf("expected 1 outbox event recorded, got %d", len(mockRepo.outboxEvents))
	}
	evt := mockRepo.outboxEvents[0]
	if evt.EventType != "UserLoggedIn" {
		t.Errorf("expected event_type UserLoggedIn, got %s", evt.EventType)
	}
	if evt.AggregateID != userID.String() {
		t.Errorf("expected aggregate_id %s, got %s", userID.String(), evt.AggregateID)
	}
}

func TestLogin_InvalidEmailOrPassword_GenericResponse(t *testing.T) {
	svc, mockRepo, _ := setupTestService()

	rawPassword := "StrongPassword123!"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)
	mockRepo.byEmail["user@example.com"] = &model.AuthCredential{
		ID:           uuid.Must(uuid.NewV7()),
		UserID:       uuid.Must(uuid.NewV7()),
		Email:        "user@example.com",
		PasswordHash: string(hashedPassword),
		Role:         model.RoleCustomer,
		IsActive:     true,
	}

	tests := []struct {
		name     string
		email    string
		password string
	}{
		{"Non existent email", "unknown@example.com", "StrongPassword123!"},
		{"Incorrect password", "user@example.com", "WrongPassword999!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Login(context.Background(), &dto.LoginRequest{
				Email:    tt.email,
				Password: tt.password,
			})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			appErr := appErrors.AsAppError(err)
			if appErr.HTTPStatus != 401 {
				t.Errorf("expected status 401, got %d", appErr.HTTPStatus)
			}
			if appErr.ClientMessage() != "Invalid email or password" {
				t.Errorf("expected generic error message 'Invalid email or password', got '%s'", appErr.ClientMessage())
			}
		})
	}
}

func TestLogin_InactiveOrLockedAccount(t *testing.T) {
	svc, mockRepo, _ := setupTestService()

	rawPassword := "StrongPassword123!"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	// Inactive account
	inactiveID := uuid.Must(uuid.NewV7())
	mockRepo.byEmail["inactive@example.com"] = &model.AuthCredential{
		ID:           inactiveID,
		UserID:       uuid.Must(uuid.NewV7()),
		Email:        "inactive@example.com",
		PasswordHash: string(hashedPassword),
		IsActive:     false,
	}

	// Locked account
	lockedID := uuid.Must(uuid.NewV7())
	futureLock := time.Now().Add(10 * time.Minute)
	mockRepo.byEmail["locked@example.com"] = &model.AuthCredential{
		ID:           lockedID,
		UserID:       uuid.Must(uuid.NewV7()),
		Email:        "locked@example.com",
		PasswordHash: string(hashedPassword),
		IsActive:     true,
		LockedUntil:  &futureLock,
	}

	_, err := svc.Login(context.Background(), &dto.LoginRequest{
		Email:    "inactive@example.com",
		Password: rawPassword,
	})
	if err == nil {
		t.Error("expected error for inactive account")
	} else {
		appErr := appErrors.AsAppError(err)
		if appErr.HTTPStatus != 401 {
			t.Errorf("expected 401 for inactive account, got %d", appErr.HTTPStatus)
		}
	}

	_, err = svc.Login(context.Background(), &dto.LoginRequest{
		Email:    "locked@example.com",
		Password: rawPassword,
	})
	if err == nil {
		t.Error("expected error for locked account")
	} else {
		appErr := appErrors.AsAppError(err)
		if appErr.HTTPStatus != 401 {
			t.Errorf("expected 401 for locked account, got %d", appErr.HTTPStatus)
		}
	}
}

func TestLogin_LockoutAfterFailedAttempts(t *testing.T) {
	svc, mockRepo, _ := setupTestService()

	credID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())
	rawPassword := "StrongPassword123!"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	mockRepo.byEmail["user@example.com"] = &model.AuthCredential{
		ID:               credID,
		UserID:           userID,
		Email:            "user@example.com",
		PasswordHash:     string(hashedPassword),
		IsActive:         true,
		FailedLoginCount: 4, // 4 failed logins already
	}

	// 5th failed attempt should trigger lock
	_, err := svc.Login(context.Background(), &dto.LoginRequest{
		Email:    "user@example.com",
		Password: "WrongPassword!",
	})
	if err == nil {
		t.Fatal("expected error on failed login")
	}

	if mockRepo.failedCountMap[credID] != 5 {
		t.Errorf("expected failed_login_count to be 5, got %d", mockRepo.failedCountMap[credID])
	}
	if mockRepo.lockedUntilMap[credID] == nil || time.Now().After(*mockRepo.lockedUntilMap[credID]) {
		t.Error("expected locked_until to be set in future")
	}
}

func TestRegister_Customer_Success(t *testing.T) {
	svc, mockRepo, _ := setupTestService()

	req := &dto.RegisterRequest{
		Email:      "customer@example.com",
		Password:   "Password123!",
		FirstName:  "John",
		LastName:   "Doe",
		IsMerchant: false,
	}

	resp, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("expected successful registration, got err: %v", err)
	}

	if resp.AccessToken == "" || resp.RefreshToken == "" || resp.UserID == "" {
		t.Errorf("expected populated response fields, got %+v", resp)
	}

	// Verify credential in mock repo has role CUSTOMER
	cred := mockRepo.byEmail["customer@example.com"]
	if cred == nil {
		t.Fatal("expected credential stored in repository")
	}
	if cred.Role != model.RoleCustomer {
		t.Errorf("expected role %s, got %s", model.RoleCustomer, cred.Role)
	}

	// Verify outbox event
	if len(mockRepo.outboxEvents) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(mockRepo.outboxEvents))
	}
	evt := mockRepo.outboxEvents[0]
	if evt.EventType != "UserRegistered" {
		t.Errorf("expected EventType UserRegistered, got %s", evt.EventType)
	}
	if evt.Topic != "auth.user.registered" {
		t.Errorf("expected Topic auth.user.registered, got %s", evt.Topic)
	}
}

func TestRegister_Merchant_Success(t *testing.T) {
	svc, mockRepo, _ := setupTestService()

	req := &dto.RegisterRequest{
		Email:      "merchant@example.com",
		Password:   "Password123!",
		FirstName:  "Jane",
		LastName:   "Merchant",
		IsMerchant: true,
	}

	resp, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("expected successful registration, got err: %v", err)
	}

	if resp.AccessToken == "" || resp.RefreshToken == "" || resp.UserID == "" {
		t.Errorf("expected populated response fields, got %+v", resp)
	}

	// Verify credential in mock repo has role MERCHANT
	cred := mockRepo.byEmail["merchant@example.com"]
	if cred == nil {
		t.Fatal("expected credential stored in repository")
	}
	if cred.Role != model.RoleMerchant {
		t.Errorf("expected role %s, got %s", model.RoleMerchant, cred.Role)
	}

	// Verify outbox event
	if len(mockRepo.outboxEvents) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(mockRepo.outboxEvents))
	}
	evt := mockRepo.outboxEvents[0]
	if evt.EventType != "MerchantRegistered" {
		t.Errorf("expected EventType MerchantRegistered, got %s", evt.EventType)
	}
	if evt.Topic != "auth.merchant.registered" {
		t.Errorf("expected Topic auth.merchant.registered, got %s", evt.Topic)
	}
}

func TestRegister_CompoundUniqueness_SameEmailBothRoles(t *testing.T) {
	svc, _, _ := setupTestService()

	email := "shared@example.com"

	// 1. Register as CUSTOMER -> should succeed
	custReq := &dto.RegisterRequest{
		Email:      email,
		Password:   "Password123!",
		IsMerchant: false,
	}
	custResp, err := svc.Register(context.Background(), custReq)
	if err != nil {
		t.Fatalf("expected customer registration to succeed, got: %v", err)
	}

	// 2. Register same email as MERCHANT -> should also succeed
	merchReq := &dto.RegisterRequest{
		Email:      email,
		Password:   "Password123!",
		IsMerchant: true,
	}
	merchResp, err := svc.Register(context.Background(), merchReq)
	if err != nil {
		t.Fatalf("expected merchant registration with same email to succeed, got: %v", err)
	}

	if custResp.UserID == merchResp.UserID {
		t.Error("expected different user IDs for customer and merchant credentials")
	}

	// 3. Registering again as CUSTOMER -> should fail with 409 Conflict
	_, err = svc.Register(context.Background(), custReq)
	if err == nil {
		t.Fatal("expected conflict error when re-registering as customer")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.HTTPStatus != 409 {
		t.Errorf("expected HTTP 409 Conflict, got %d", appErr.HTTPStatus)
	}

	// 4. Registering again as MERCHANT -> should fail with 409 Conflict
	_, err = svc.Register(context.Background(), merchReq)
	if err == nil {
		t.Fatal("expected conflict error when re-registering as merchant")
	}
	appErr = appErrors.AsAppError(err)
	if appErr.HTTPStatus != 409 {
		t.Errorf("expected HTTP 409 Conflict, got %d", appErr.HTTPStatus)
	}
}


type mockUserServiceClient struct {
	createdUsers []*userpb.CreateUserRequest
	createErr    error
}

func (m *mockUserServiceClient) CreateUser(ctx context.Context, req *userpb.CreateUserRequest, opts ...grpc.CallOption) (*userpb.CreateUserResponse, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.createdUsers = append(m.createdUsers, req)
	return &userpb.CreateUserResponse{
		User: &userpb.User{
			Id:        req.Id,
			Email:     req.Email,
			FirstName: req.FirstName,
			LastName:  req.LastName,
		},
	}, nil
}

func (m *mockUserServiceClient) GetUser(ctx context.Context, in *userpb.GetUserRequest, opts ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) UpdateUser(ctx context.Context, in *userpb.UpdateUserRequest, opts ...grpc.CallOption) (*userpb.UpdateUserResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) CreateUserAddress(ctx context.Context, in *userpb.CreateUserAddressRequest, opts ...grpc.CallOption) (*userpb.CreateUserAddressResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) ListUserAddresses(ctx context.Context, in *userpb.ListUserAddressesRequest, opts ...grpc.CallOption) (*userpb.ListUserAddressesResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) GetUserAddress(ctx context.Context, in *userpb.GetUserAddressRequest, opts ...grpc.CallOption) (*userpb.GetUserAddressResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) UpdateUserAddress(ctx context.Context, in *userpb.UpdateUserAddressRequest, opts ...grpc.CallOption) (*userpb.UpdateUserAddressResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) DeleteUserAddress(ctx context.Context, in *userpb.DeleteUserAddressRequest, opts ...grpc.CallOption) (*userpb.DeleteUserAddressResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) SetDefaultUserAddress(ctx context.Context, in *userpb.SetDefaultUserAddressRequest, opts ...grpc.CallOption) (*userpb.SetDefaultUserAddressResponse, error) {
	return nil, nil
}

func TestRegister_SuccessWithUserService(t *testing.T) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-12345",
			ExpiryMinutes: 15,
		},
	}
	mockUserClient := &mockUserServiceClient{}
	svc := NewAuthService(mockRepo, cfg, nil, mockUserClient)

	req := &dto.RegisterRequest{
		Email:     "newuser@example.com",
		Password:  "StrongPassword123!",
		FirstName: "John",
		LastName:  "Doe",
	}

	resp, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("expected successful registration, got err: %v", err)
	}

	if resp.AccessToken == "" || resp.UserID == "" {
		t.Error("expected non-empty access token and user ID")
	}

	if len(mockUserClient.createdUsers) != 1 {
		t.Fatalf("expected 1 call to user service CreateUser, got %d", len(mockUserClient.createdUsers))
	}

	created := mockUserClient.createdUsers[0]
	if created.Email != req.Email {
		t.Errorf("expected email %s, got %s", req.Email, created.Email)
	}
	if created.FirstName != "John" || created.LastName != "Doe" {
		t.Errorf("expected name John Doe, got %s %s", created.FirstName, created.LastName)
	}
	if created.Id != resp.UserID {
		t.Errorf("expected gRPC user id %s to match response user id %s", created.Id, resp.UserID)
	}
}

func TestRegister_UserServiceUnavailable(t *testing.T) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-12345",
			ExpiryMinutes: 15,
		},
	}
	svc := NewAuthService(mockRepo, cfg, nil)

	req := &dto.RegisterRequest{
		Email:     "newuser@example.com",
		Password:  "StrongPassword123!",
		FirstName: "John",
		LastName:  "Doe",
	}

	_, err := svc.Register(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when userClient is nil, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeServiceUnavailable {
		t.Fatalf("expected ServiceUnavailable code, got: %s", appErr.Code)
	}
}

