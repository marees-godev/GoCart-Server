package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofrs/uuid/v5"

	"github.com/golang-jwt/jwt/v5"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	pkgredis "github.com/marees-godev/GoCart-Server/pkg/redis"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/otp"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/repository"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type mockAuthRepository struct {
	byEmail            map[string]*model.AuthCredential
	byEmailRole        map[string]*model.AuthCredential
	failedCountMap     map[uuid.UUID]int
	lockedUntilMap     map[uuid.UUID]*time.Time
	refreshTokens      []*model.RefreshToken
	outboxEvents       []*outbox.Event
	getByEmailErr      error
	createSessionErr   error
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
		if c, found := m.byEmail[email]; found && (c.Role == role || c.Role == "") {
			cred = c
		} else {
			return nil, repository.ErrNotFound
		}
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
	filtered := make([]*model.RefreshToken, 0, len(m.refreshTokens))
	for _, rt := range m.refreshTokens {
		if rt.UserID != refreshToken.UserID {
			filtered = append(filtered, rt)
		}
	}
	m.refreshTokens = append(filtered, refreshToken)
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
	for email, cred := range m.byEmail {
		if cred.ID == id {
			delete(m.byEmail, email)
			delete(m.byEmailRole, cred.Email+":"+cred.Role.String())
			break
		}
	}
	return nil
}


func (m *mockAuthRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*model.AuthCredential, error) {
	for _, cred := range m.byEmail {
		if cred.UserID == userID {
			return cred, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *mockAuthRepository) GetRefreshToken(ctx context.Context, tokenHash string, userIDOrEmail string) (*model.RefreshToken, error) {
	for _, rt := range m.refreshTokens {
		if rt.TokenHash == tokenHash {
			if userIDOrEmail != "" {
				cred, _ := m.GetByUserID(ctx, rt.UserID)
				if rt.UserID.String() != userIDOrEmail && (cred == nil || cred.Email != userIDOrEmail) {
					continue
				}
			}
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

func (m *mockAuthRepository) RevokeRefreshTokenByHash(ctx context.Context, tokenHash string) error {
	for _, rt := range m.refreshTokens {
		if rt.TokenHash == tokenHash {
			rt.Revoked = true
		}
	}
	return nil
}

func (m *mockAuthRepository) RevokeRefreshTokensByUserID(ctx context.Context, userID uuid.UUID) error {
	for _, rt := range m.refreshTokens {
		if rt.UserID == userID {
			rt.Revoked = true
		}
	}
	return nil
}

func (m *mockAuthRepository) RotateRefreshToken(ctx context.Context, oldTokenID uuid.UUID, newToken *model.RefreshToken) error {
	for _, rt := range m.refreshTokens {
		if rt.ID == oldTokenID {
			rt.Revoked = true
		}
	}
	m.refreshTokens = append(m.refreshTokens, newToken)
	return nil
}

func (m *mockAuthRepository) MarkEmailVerified(ctx context.Context, userID uuid.UUID) error {
	for _, cred := range m.byEmail {
		if cred.UserID == userID {
			cred.EmailVerified = true
		}
	}
	return nil
}

func (m *mockAuthRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string, evt *outbox.Event) error {
	for _, cred := range m.byEmail {
		if cred.UserID == userID {
			cred.PasswordHash = passwordHash
			cred.FailedLoginCount = 0
			cred.LockedUntil = nil
			if evt != nil {
				m.outboxEvents = append(m.outboxEvents, evt)
			}
			for _, rt := range m.refreshTokens {
				if rt.UserID == userID {
					rt.Revoked = true
				}
			}
			return nil
		}
	}
	return repository.ErrNotFound
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

func setupTestServiceWithOTPStore() (AuthService, *mockAuthRepository, *config.Config, otp.Store) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-12345",
			ExpiryMinutes: 15,
		},
	}
	store := otp.NewMemoryStore()
	svc := NewAuthService(mockRepo, cfg, nil, &mockUserServiceClient{}, store)
	return svc, mockRepo, cfg, store
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
		EmailVerified:    true,
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

func TestLogin_Merchant_Success(t *testing.T) {
	svc, mockRepo, cfg := setupTestService()
	_ = cfg

	userID := uuid.Must(uuid.NewV7())
	credID := uuid.Must(uuid.NewV7())
	rawPassword := "MerchantPass123!"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	mockRepo.byEmailRole["merchant@example.com:MERCHANT"] = &model.AuthCredential{
		ID:           credID,
		UserID:       userID,
		Email:        "merchant@example.com",
		PasswordHash: string(hashedPassword),
		Role:         model.RoleMerchant,
		EmailVerified: true,
		IsActive:     true,
	}

	req := &dto.LoginRequest{
		Email:      "merchant@example.com",
		Password:   rawPassword,
		IsMerchant: true,
	}

	resp, err := svc.Login(context.Background(), req)
	if err != nil {
		t.Fatalf("expected successful merchant login, got error: %v", err)
	}

	if resp.Role != model.RoleMerchant.String() {
		t.Errorf("expected role MERCHANT, got %s", resp.Role)
	}
	if resp.UserID != userID.String() {
		t.Errorf("expected userID %s, got %s", userID.String(), resp.UserID)
	}
}

func TestLogin_RoleMismatch_Fails(t *testing.T) {
	svc, mockRepo, _ := setupTestService()

	rawPassword := "SecretPass123!"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	// User exists ONLY as MERCHANT
	mockRepo.byEmailRole["seller@example.com:MERCHANT"] = &model.AuthCredential{
		ID:           uuid.Must(uuid.NewV7()),
		UserID:       uuid.Must(uuid.NewV7()),
		Email:        "seller@example.com",
		PasswordHash: string(hashedPassword),
		Role:         model.RoleMerchant,
		IsActive:     true,
	}

	// Attempt to login as CUSTOMER (IsMerchant: false)
	_, err := svc.Login(context.Background(), &dto.LoginRequest{
		Email:      "seller@example.com",
		Password:   rawPassword,
		IsMerchant: false,
	})
	if err == nil {
		t.Fatal("expected error when trying to login as customer with merchant-only account")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.HTTPStatus != 401 {
		t.Errorf("expected 401, got %d", appErr.HTTPStatus)
	}
}

func TestLogin_UnverifiedEmail_Fails(t *testing.T) {
	svc, mockRepo, _ := setupTestService()

	rawPassword := "ValidPassword123!"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	mockRepo.byEmailRole["unverified@example.com:CUSTOMER"] = &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        uuid.Must(uuid.NewV7()),
		Email:         "unverified@example.com",
		PasswordHash:  string(hashedPassword),
		Role:          model.RoleCustomer,
		EmailVerified: false,
		IsActive:      true,
	}

	_, err := svc.Login(context.Background(), &dto.LoginRequest{
		Email:      "unverified@example.com",
		Password:   rawPassword,
		IsMerchant: false,
	})
	if err == nil {
		t.Fatal("expected error when trying to login with unverified email, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected error code FORBIDDEN, got %s", appErr.Code)
	}
	if appErr.HTTPStatus != 403 {
		t.Errorf("expected status 403, got %d", appErr.HTTPStatus)
	}
	expectedMsg := "email is not verified, please verify your email first and then login"
	if appErr.ClientMessage() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, appErr.ClientMessage())
	}
}

func TestLogin_MultiRoleSameEmail_SeparatesAccount(t *testing.T) {
	svc, mockRepo, _ := setupTestService()

	rawPassword := "SharedPass123!"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	customerUserID := uuid.Must(uuid.NewV7())
	merchantUserID := uuid.Must(uuid.NewV7())

	mockRepo.byEmailRole["dual@example.com:CUSTOMER"] = &model.AuthCredential{
		ID:           uuid.Must(uuid.NewV7()),
		UserID:       customerUserID,
		Email:        "dual@example.com",
		PasswordHash: string(hashedPassword),
		Role:         model.RoleCustomer,
		EmailVerified: true,
		IsActive:     true,
	}

	mockRepo.byEmailRole["dual@example.com:MERCHANT"] = &model.AuthCredential{
		ID:           uuid.Must(uuid.NewV7()),
		UserID:       merchantUserID,
		Email:        "dual@example.com",
		PasswordHash: string(hashedPassword),
		Role:         model.RoleMerchant,
		EmailVerified: true,
		IsActive:     true,
	}

	// Login as customer
	cResp, err := svc.Login(context.Background(), &dto.LoginRequest{
		Email:      "dual@example.com",
		Password:   rawPassword,
		IsMerchant: false,
	})
	if err != nil {
		t.Fatalf("customer login failed: %v", err)
	}
	if cResp.UserID != customerUserID.String() || cResp.Role != "CUSTOMER" {
		t.Errorf("expected customer account %s / CUSTOMER, got %s / %s", customerUserID.String(), cResp.UserID, cResp.Role)
	}

	// Login as merchant
	mResp, err := svc.Login(context.Background(), &dto.LoginRequest{
		Email:      "dual@example.com",
		Password:   rawPassword,
		IsMerchant: true,
	})
	if err != nil {
		t.Fatalf("merchant login failed: %v", err)
	}
	if mResp.UserID != merchantUserID.String() || mResp.Role != "MERCHANT" {
		t.Errorf("expected merchant account %s / MERCHANT, got %s / %s", merchantUserID.String(), mResp.UserID, mResp.Role)
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
		Role:          model.RoleCustomer,
		EmailVerified: true,
		IsActive:      true,
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
		EmailVerified: true,
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
		EmailVerified: true,
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
		EmailVerified:    true,
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

func TestVerifyEmail_Flow(t *testing.T) {
	svc, mockRepo, _, otpStore := setupTestServiceWithOTPStore()

	userID := uuid.Must(uuid.NewV7())
	mockRepo.byEmail["unverified@example.com"] = &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        userID,
		Email:         "unverified@example.com",
		Role:          model.RoleCustomer,
		EmailVerified: false,
		IsActive:      true,
	}

	rawOTP := "847291"
	_ = otpStore.SetOTP(context.Background(), "unverified@example.com", hashToken(rawOTP), 5*time.Minute)

	// 1. Verify with valid Email and OTP
	res, err := svc.VerifyEmail(context.Background(), &dto.VerifyEmailRequest{
		Email: "unverified@example.com",
		OTP:   rawOTP,
	})
	if err != nil {
		t.Fatalf("expected successful email verification, got: %v", err)
	}
	if !res.Success {
		t.Error("expected res.Success to be true")
	}

	// Verify user email_verified updated to true
	cred := mockRepo.byEmail["unverified@example.com"]
	if !cred.EmailVerified {
		t.Error("expected credential EmailVerified to be true after verification")
	}

	// OTP should be deleted from store after successful verification (one-time use)
	_, err = otpStore.GetOTP(context.Background(), "unverified@example.com")
	if err == nil {
		t.Error("expected OTP to be deleted from store after successful verification")
	}

	// 2. Verify already verified email -> should return success
	resAlready, err := svc.VerifyEmail(context.Background(), &dto.VerifyEmailRequest{
		Email: "unverified@example.com",
		OTP:   rawOTP,
	})
	if err != nil || !resAlready.Success {
		t.Errorf("expected success for already verified email, got err: %v", err)
	}

	// 3. New unverified user with invalid OTP -> should fail
	user2ID := uuid.Must(uuid.NewV7())
	mockRepo.byEmail["user2@example.com"] = &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        user2ID,
		Email:         "user2@example.com",
		Role:          model.RoleCustomer,
		EmailVerified: false,
		IsActive:      true,
	}
	_ = otpStore.SetOTP(context.Background(), "user2@example.com", hashToken("123456"), 5*time.Minute)

	_, err = svc.VerifyEmail(context.Background(), &dto.VerifyEmailRequest{
		Email: "user2@example.com",
		OTP:   "999999", // incorrect code
	})
	if err == nil {
		t.Fatal("expected error when verifying with wrong OTP")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.HTTPStatus != 400 {
		t.Errorf("expected 400 Bad Request, got %d", appErr.HTTPStatus)
	}

	// 4. Verify with expired/non-existent OTP -> should fail
	user3ID := uuid.Must(uuid.NewV7())
	mockRepo.byEmail["user3@example.com"] = &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        user3ID,
		Email:         "user3@example.com",
		Role:          model.RoleCustomer,
		EmailVerified: false,
		IsActive:      true,
	}
	_ = otpStore.SetOTP(context.Background(), "user3@example.com", hashToken("654321"), -1*time.Minute)

	_, err = svc.VerifyEmail(context.Background(), &dto.VerifyEmailRequest{
		Email: "user3@example.com",
		OTP:   "654321",
	})
	if err == nil {
		t.Fatal("expected error when verifying with expired OTP")
	}
	appErr = appErrors.AsAppError(err)
	if appErr.HTTPStatus != 400 {
		t.Errorf("expected 400 Bad Request for expired OTP, got %d", appErr.HTTPStatus)
	}
}

func TestResendVerificationEmail(t *testing.T) {
	svc, mockRepo, _, otpStore := setupTestServiceWithOTPStore()

	userID := uuid.Must(uuid.NewV7())
	mockRepo.byEmail["resend@example.com"] = &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        userID,
		Email:         "resend@example.com",
		Role:          model.RoleCustomer,
		EmailVerified: false,
		IsActive:      true,
	}

	res, err := svc.ResendVerificationEmail(context.Background(), &dto.ResendVerificationEmailRequest{
		Email: "resend@example.com",
	})
	if err != nil {
		t.Fatalf("expected successful resend, got: %v", err)
	}
	if !res.Success {
		t.Error("expected res.Success to be true")
	}

	storedHash, err := otpStore.GetOTP(context.Background(), "resend@example.com")
	if err != nil || storedHash == "" {
		t.Errorf("expected OTP to be stored for resend@example.com, got err: %v", err)
	}
}



type mockUserServiceClient struct {
	userpb.UserServiceClient
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

type mockMerchantClient struct {
	merchantpb.MerchantServiceClient
	createdReqs []*merchantpb.CreateMerchantRequest
	capturedCtx context.Context
	createErr   error
}

func (m *mockMerchantClient) CreateMerchant(ctx context.Context, in *merchantpb.CreateMerchantRequest, opts ...grpc.CallOption) (*merchantpb.CreateMerchantResponse, error) {
	m.capturedCtx = ctx
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.createdReqs = append(m.createdReqs, in)
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

func TestRegister_CallsMerchantService_WhenIsMerchantTrue(t *testing.T) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-12345",
			ExpiryMinutes: 15,
		},
	}
	mockMerchant := &mockMerchantClient{}
	svc := NewAuthService(mockRepo, cfg, nil, &mockUserServiceClient{}, mockMerchant)

	req := &dto.RegisterRequest{
		Email:        "merchant@example.com",
		Password:     "Password123!",
		FirstName:    "Jane",
		LastName:     "Doe",
		BusinessName: "Jane's Superstore",
		IsMerchant:   true,
	}

	resp, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("expected successful registration, got: %v", err)
	}

	if len(mockMerchant.createdReqs) != 1 {
		t.Fatalf("expected 1 call to merchant service CreateMerchant, got %d", len(mockMerchant.createdReqs))
	}

	created := mockMerchant.createdReqs[0]
	if created.UserId != resp.UserID {
		t.Errorf("expected merchant ID %s, got %s", resp.UserID, created.UserId)
	}
	if created.BusinessName != "Jane's Superstore" {
		t.Errorf("expected business name Jane's Superstore, got %s", created.BusinessName)
	}
	if created.BusinessEmail != "merchant@example.com" {
		t.Errorf("expected email merchant@example.com, got %s", created.BusinessEmail)
	}
	if created.FirstName != "Jane" || created.LastName != "Doe" {
		t.Errorf("expected name Jane Doe, got %s %s", created.FirstName, created.LastName)
	}

	md, ok := metadata.FromOutgoingContext(mockMerchant.capturedCtx)
	if !ok {
		t.Fatal("expected outgoing metadata in context")
	}
	if vals := md.Get("x-user-role"); len(vals) == 0 || vals[0] != "MERCHANT" {
		t.Errorf("expected role MERCHANT in metadata, got %v", vals)
	}
	if vals := md.Get("x-user-id"); len(vals) == 0 || vals[0] != resp.UserID {
		t.Errorf("expected user id %s in metadata, got %v", resp.UserID, vals)
	}
}

func TestRegister_DoesNotCallMerchantService_WhenIsMerchantFalse(t *testing.T) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-12345",
			ExpiryMinutes: 15,
		},
	}
	mockMerchant := &mockMerchantClient{}
	svc := NewAuthService(mockRepo, cfg, nil, &mockUserServiceClient{}, mockMerchant)

	req := &dto.RegisterRequest{
		Email:      "customer@example.com",
		Password:   "Password123!",
		FirstName:  "John",
		LastName:   "Customer",
		IsMerchant: false,
	}

	_, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("expected successful customer registration, got: %v", err)
	}

	if len(mockMerchant.createdReqs) != 0 {
		t.Errorf("expected 0 calls to merchant service, got %d", len(mockMerchant.createdReqs))
	}
}

func TestRegister_ReturnsError_WhenMerchantServiceFails(t *testing.T) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-12345",
			ExpiryMinutes: 15,
		},
	}
	mockMerchant := &mockMerchantClient{
		createErr: errors.New("merchant service connection timeout"),
	}
	svc := NewAuthService(mockRepo, cfg, nil, &mockUserServiceClient{}, mockMerchant)

	req := &dto.RegisterRequest{
		Email:      "merchant@example.com",
		Password:   "Password123!",
		FirstName:  "Jane",
		LastName:   "Merchant",
		IsMerchant: true,
	}

	_, err := svc.Register(context.Background(), req)
	if err == nil {
		t.Fatal("expected registration to fail when merchant service fails, got nil")
	}
}

func TestResendVerificationEmail_Cooldown(t *testing.T) {
	svc, mockRepo, cfg, otpStore := setupTestServiceWithOTPStore()
	cfg.Email.TokenTTLMinutes = 5
	cfg.Email.ResendCooldownSeconds = 60

	// 1. User not found -> NotFound error
	_, err := svc.ResendVerificationEmail(context.Background(), &dto.ResendVerificationEmailRequest{
		Email: "nonexistent@example.com",
	})
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
	if appErr := appErrors.AsAppError(err); appErr.HTTPStatus != 404 {
		t.Errorf("expected 404 Not Found, got %d", appErr.HTTPStatus)
	}

	// 2. Already verified user -> BadRequest error
	mockRepo.byEmail["verified@example.com"] = &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        uuid.Must(uuid.NewV7()),
		Email:         "verified@example.com",
		Role:          model.RoleCustomer,
		EmailVerified: true,
		IsActive:      true,
	}
	_, err = svc.ResendVerificationEmail(context.Background(), &dto.ResendVerificationEmailRequest{
		Email: "verified@example.com",
	})
	if err == nil {
		t.Fatal("expected error for already verified user")
	}
	if appErr := appErrors.AsAppError(err); appErr.HTTPStatus != 400 {
		t.Errorf("expected 400 Bad Request, got %d", appErr.HTTPStatus)
	}

	// 3. Unverified user - first resend request -> should succeed
	mockRepo.byEmail["unverified@example.com"] = &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        uuid.Must(uuid.NewV7()),
		Email:         "unverified@example.com",
		Role:          model.RoleCustomer,
		EmailVerified: false,
		IsActive:      true,
	}

	resp, err := svc.ResendVerificationEmail(context.Background(), &dto.ResendVerificationEmailRequest{
		Email: "unverified@example.com",
	})
	if err != nil {
		t.Fatalf("expected first resend to succeed, got: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success true, got false")
	}

	// Verify an OTP was saved in store
	firstHash, err := otpStore.GetOTP(context.Background(), "unverified@example.com")
	if err != nil || firstHash == "" {
		t.Fatalf("expected OTP hash to be stored, got err: %v", err)
	}

	// 4. Immediate second resend (within 60s cooldown) -> should be rate-limited with 429
	_, err = svc.ResendVerificationEmail(context.Background(), &dto.ResendVerificationEmailRequest{
		Email: "unverified@example.com",
	})
	if err == nil {
		t.Fatal("expected cooldown rate-limit error on immediate resend, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.HTTPStatus != 429 {
		t.Errorf("expected 429 Too Many Requests, got %d", appErr.HTTPStatus)
	}
	if appErr.Code != appErrors.CodeTooManyRequests {
		t.Errorf("expected CodeTooManyRequests, got %s", appErr.Code)
	}

	// 5. Simulate passage of cooldown: set TTL to remaining 3m50s (60s+ has elapsed from 5m total TTL)
	_ = otpStore.SetOTP(context.Background(), "unverified@example.com", firstHash, 3*time.Minute+50*time.Second)

	resp, err = svc.ResendVerificationEmail(context.Background(), &dto.ResendVerificationEmailRequest{
		Email: "unverified@example.com",
	})
	if err != nil {
		t.Fatalf("expected resend to succeed after cooldown period elapsed, got: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success true, got false")
	}
}

func TestOTP_CaseInsensitiveEmailHandling(t *testing.T) {
	svc, mockRepo, _, otpStore := setupTestServiceWithOTPStore()

	rawOTP := "882244"
	userEmail := "case.Test@Example.COM"
	normalizedEmail := "case.test@example.com"

	mockRepo.byEmail[normalizedEmail] = &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        uuid.Must(uuid.NewV7()),
		Email:         normalizedEmail,
		Role:          model.RoleCustomer,
		EmailVerified: false,
		IsActive:      true,
	}

	// 1. Set OTP with mixed-case and padded whitespace
	err := otpStore.SetOTP(context.Background(), "  "+userEmail+"  ", hashToken(rawOTP), 5*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error setting OTP: %v", err)
	}

	// 2. Lookup OTP directly with normalized lowercase email
	hash, err := otpStore.GetOTP(context.Background(), normalizedEmail)
	if err != nil {
		t.Fatalf("expected to find OTP using normalized email, got: %v", err)
	}
	if hash != hashToken(rawOTP) {
		t.Errorf("hash mismatch: expected %s, got %s", hashToken(rawOTP), hash)
	}

	// 3. VerifyEmail with uppercase and whitespace input -> should succeed
	verifyResp, err := svc.VerifyEmail(context.Background(), &dto.VerifyEmailRequest{
		Email: "   CASE.TEST@EXAMPLE.COM   ",
		OTP:   rawOTP,
	})
	if err != nil {
		t.Fatalf("expected verification to succeed with mixed-case email, got: %v", err)
	}
	if !verifyResp.Success {
		t.Errorf("expected verification success to be true")
	}

	// 4. Verify OTP was deleted from store using mixed-case query
	_, err = otpStore.GetOTP(context.Background(), "Case.Test@Example.Com")
	if !errors.Is(err, otp.ErrOTPNotFound) {
		t.Errorf("expected ErrOTPNotFound after deletion, got: %v", err)
	}
}

func TestRefreshToken_Success_And_Rotation(t *testing.T) {
	svc, mockRepo, _ := setupTestService()

	userID := uuid.Must(uuid.NewV7())
	userEmail := "refresh.user@example.com"
	mockRepo.byEmail[userEmail] = &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        userID,
		Email:         userEmail,
		Role:          model.RoleCustomer,
		EmailVerified: true,
		IsActive:      true,
	}

	rawInitialToken := "valid-initial-refresh-token-12345"
	initialTokenHash := hashToken(rawInitialToken)
	initialTokenModel := &model.RefreshToken{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		TokenHash: initialTokenHash,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		Revoked:   false,
	}
	mockRepo.refreshTokens = append(mockRepo.refreshTokens, initialTokenModel)

	resp, err := svc.RefreshToken(context.Background(), &dto.RefreshTokenRequest{
		RefreshToken: rawInitialToken,
	})
	if err != nil {
		t.Fatalf("expected successful refresh, got: %v", err)
	}

	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if resp.RefreshToken == "" || resp.RefreshToken == rawInitialToken {
		t.Errorf("expected new rotated refresh token, got %s", resp.RefreshToken)
	}
	if resp.UserID != userID.String() {
		t.Errorf("expected user_id %s, got %s", userID.String(), resp.UserID)
	}

	// Verify old initial refresh token is marked revoked
	if !initialTokenModel.Revoked {
		t.Error("expected initial refresh token to be revoked after rotation")
	}

	// Verify new rotated refresh token exists in repository
	rotatedHash := hashToken(resp.RefreshToken)
	foundRotated := false
	for _, rt := range mockRepo.refreshTokens {
		if rt.TokenHash == rotatedHash {
			foundRotated = true
			if rt.Revoked {
				t.Error("expected new rotated token to be active, not revoked")
			}
			if rt.ExpiresAt.Before(time.Now().Add(6 * 24 * time.Hour)) {
				t.Error("expected new rotated token to have 7-day expiration")
			}
		}
	}
	if !foundRotated {
		t.Error("expected new rotated refresh token to be stored in repository")
	}

	// Reusing rotated/revoked initial refresh token must fail
	_, err = svc.RefreshToken(context.Background(), &dto.RefreshTokenRequest{
		RefreshToken: rawInitialToken,
	})
	if err == nil {
		t.Error("expected reuse of rotated refresh token to be rejected")
	}
}

func TestRefreshToken_ExpiredToken_Rejected(t *testing.T) {
	svc, mockRepo, _ := setupTestService()

	userID := uuid.Must(uuid.NewV7())
	mockRepo.byEmail["expired@example.com"] = &model.AuthCredential{
		ID:       uuid.Must(uuid.NewV7()),
		UserID:   userID,
		Email:    "expired@example.com",
		Role:     model.RoleCustomer,
		IsActive: true,
	}

	rawExpiredToken := "expired-refresh-token-999"
	mockRepo.refreshTokens = append(mockRepo.refreshTokens, &model.RefreshToken{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		TokenHash: hashToken(rawExpiredToken),
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
		Revoked:   false,
	})

	_, err := svc.RefreshToken(context.Background(), &dto.RefreshTokenRequest{
		RefreshToken: rawExpiredToken,
	})
	if err == nil {
		t.Error("expected expired refresh token to be rejected")
	}
}

func TestRefreshToken_RevokedToken_Rejected(t *testing.T) {
	svc, mockRepo, _ := setupTestService()

	userID := uuid.Must(uuid.NewV7())
	mockRepo.byEmail["revoked@example.com"] = &model.AuthCredential{
		ID:       uuid.Must(uuid.NewV7()),
		UserID:   userID,
		Email:    "revoked@example.com",
		Role:     model.RoleCustomer,
		IsActive: true,
	}

	rawRevokedToken := "revoked-refresh-token-888"
	mockRepo.refreshTokens = append(mockRepo.refreshTokens, &model.RefreshToken{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		TokenHash: hashToken(rawRevokedToken),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Revoked:   true,
	})

	_, err := svc.RefreshToken(context.Background(), &dto.RefreshTokenRequest{
		RefreshToken: rawRevokedToken,
	})
	if err == nil {
		t.Error("expected revoked refresh token to be rejected")
	}
}

func TestRefreshToken_InvalidToken_Rejected(t *testing.T) {
	svc, _, _ := setupTestService()

	_, err := svc.RefreshToken(context.Background(), &dto.RefreshTokenRequest{
		RefreshToken: "non-existent-token-abc",
	})
	if err == nil {
		t.Error("expected invalid refresh token to be rejected")
	}
}

func TestRefreshToken_DeactivatedUser_Rejected(t *testing.T) {
	svc, mockRepo, _ := setupTestService()

	userID := uuid.Must(uuid.NewV7())
	mockRepo.byEmail["deactivated@example.com"] = &model.AuthCredential{
		ID:       uuid.Must(uuid.NewV7()),
		UserID:   userID,
		Email:    "deactivated@example.com",
		Role:     model.RoleCustomer,
		IsActive: false, // Deactivated!
	}

	rawToken := "deactivated-user-token-777"
	mockRepo.refreshTokens = append(mockRepo.refreshTokens, &model.RefreshToken{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		TokenHash: hashToken(rawToken),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Revoked:   false,
	})

	_, err := svc.RefreshToken(context.Background(), &dto.RefreshTokenRequest{
		RefreshToken: rawToken,
	})
	if err == nil {
		t.Error("expected deactivated user session refresh to be rejected")
	}
}

func TestRefreshToken_MissingToken_Rejected(t *testing.T) {
	svc, _, _ := setupTestService()

	_, err := svc.RefreshToken(context.Background(), &dto.RefreshTokenRequest{
		RefreshToken: "",
	})
	if err == nil {
		t.Error("expected empty refresh token to be rejected")
	}

	_, err = svc.RefreshToken(context.Background(), nil)
	if err == nil {
		t.Error("expected nil request to be rejected")
	}
}

func TestRefreshToken_WithAccessTokenJWT(t *testing.T) {
	svc, mockRepo, cfg := setupTestService()

	userID := uuid.Must(uuid.NewV7())
	mockRepo.byEmail["jwtuser@example.com"] = &model.AuthCredential{
		ID:       uuid.Must(uuid.NewV7()),
		UserID:   userID,
		Email:    "jwtuser@example.com",
		Role:     model.RoleCustomer,
		IsActive: true,
	}

	rawToken := "jwt-refresh-token-999"
	mockRepo.refreshTokens = append(mockRepo.refreshTokens, &model.RefreshToken{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		TokenHash: hashToken(rawToken),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Revoked:   false,
	})

	accessToken, err := auth.GenerateToken(auth.UserContext{
		UserID: userID.String(),
		Email:  "jwtuser@example.com",
		Role:   "CUSTOMER",
	}, cfg.JWT.Secret, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate access token: %v", err)
	}

	resp, err := svc.RefreshToken(context.Background(), &dto.RefreshTokenRequest{
		RefreshToken: rawToken,
		AccessToken:  accessToken,
	})
	if err != nil {
		t.Fatalf("expected successful refresh with access token, got: %v", err)
	}

	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("expected new access and refresh tokens, got empty")
	}
}

func TestLogout_Success_RevokesRefreshToken(t *testing.T) {
	svc, mockRepo, cfg := setupTestService()

	userID := uuid.Must(uuid.NewV7())
	mockRepo.byEmail["logoutuser@example.com"] = &model.AuthCredential{
		ID:       uuid.Must(uuid.NewV7()),
		UserID:   userID,
		Email:    "logoutuser@example.com",
		Role:     model.RoleCustomer,
		IsActive: true,
	}

	rawToken := "valid-refresh-token-logout-123"
	mockRepo.refreshTokens = append(mockRepo.refreshTokens, &model.RefreshToken{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		TokenHash: hashToken(rawToken),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Revoked:   false,
	})

	accessToken, _ := auth.GenerateToken(auth.UserContext{
		UserID: userID.String(),
		Email:  "logoutuser@example.com",
		Role:   "CUSTOMER",
	}, cfg.JWT.Secret, 15*time.Minute)

	// 1. Authenticated user calls logout with AccessToken
	logoutResp, err := svc.Logout(context.Background(), &dto.LogoutRequest{
		AccessToken: accessToken,
	})
	if err != nil {
		t.Fatalf("expected logout to succeed, got: %v", err)
	}
	if !logoutResp.Success {
		t.Errorf("expected logout response success true, got false")
	}

	// 2. Try to refresh token with the revoked refresh token
	_, err = svc.RefreshToken(context.Background(), &dto.RefreshTokenRequest{
		RefreshToken: rawToken,
		AccessToken:  accessToken,
	})
	if err == nil {
		t.Fatalf("expected RefreshToken to fail for revoked token, but it succeeded")
	}

	// 3. Test Idempotency: Calling logout again with same access token succeeds
	logoutResp2, err := svc.Logout(context.Background(), &dto.LogoutRequest{
		AccessToken: accessToken,
	})
	if err != nil {
		t.Fatalf("expected repeated logout to succeed idempotently, got: %v", err)
	}
	if !logoutResp2.Success {
		t.Errorf("expected repeated logout success true, got false")
	}
}

func TestLogout_Unauthenticated_NoToken(t *testing.T) {
	svc, _, _ := setupTestService()

	_, err := svc.Logout(context.Background(), &dto.LogoutRequest{})
	if err == nil {
		t.Errorf("expected error when logging out without auth context or identity")
	}
}

// ---------------------------------------------------------------------------
// Failed Login Protection Tests
// ---------------------------------------------------------------------------

func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *pkgredis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)

	cfg := pkgredis.Config{
		Host:        mr.Host(),
		Port:        mr.Port(),
		DialTimeout: 1 * time.Second,
	}

	client, err := pkgredis.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to create redis client for test: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	return mr, client
}

func createTestUserCredential(email, password string) (*model.AuthCredential, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	userID := uuid.Must(uuid.NewV7())
	return &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        userID,
		Email:         email,
		PasswordHash:  string(hashed),
		Role:          model.RoleCustomer,
		EmailVerified: true,
		IsActive:      true,
	}, nil
}

func TestFailedLoginProtection_RedisTrackingAndLocking(t *testing.T) {
	mr, redisClient := setupTestRedis(t)

	email := "victim@example.com"
	password := "CorrectPassword123!"
	cred, err := createTestUserCredential(email, password)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	repo := newMockAuthRepository()
	_ = repo.CreateCredential(context.Background(), cred)

	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "test-secret", ExpiryMinutes: 60},
		Security: config.SecurityConfig{
			MaxLoginAttempts:    3,
			LoginAttemptWindow:  5 * time.Minute,
			AccountLockDuration: 10 * time.Minute,
		},
	}

	svc := NewAuthService(repo, cfg, nil, redisClient)
	ctx := context.Background()

	// Attempt 1: Wrong password
	_, err = svc.Login(ctx, &dto.LoginRequest{Email: email, Password: "WrongPassword1"})
	if err == nil {
		t.Fatal("expected error on attempt 1, got nil")
	}

	failedKey := "auth:login:failed:" + email
	if val, err := mr.Get(failedKey); err != nil || val != "1" {
		t.Errorf("expected failed count 1 in Redis, got val=%s err=%v", val, err)
	}

	// Attempt 2: Wrong password
	_, err = svc.Login(ctx, &dto.LoginRequest{Email: email, Password: "WrongPassword2"})
	if err == nil {
		t.Fatal("expected error on attempt 2, got nil")
	}
	if val, err := mr.Get(failedKey); err != nil || val != "2" {
		t.Errorf("expected failed count 2 in Redis, got val=%s err=%v", val, err)
	}

	// Attempt 3: Wrong password -> Reaches threshold 3 -> Account Locked
	_, err = svc.Login(ctx, &dto.LoginRequest{Email: email, Password: "WrongPassword3"})
	if err == nil {
		t.Fatal("expected error on attempt 3, got nil")
	}

	lockKey := "auth:login:locked:" + email
	if !mr.Exists(lockKey) {
		t.Error("expected Redis lock key to exist after reaching threshold")
	}

	// Attempt 4: Try logging in with CORRECT password while locked -> Must fail!
	_, err = svc.Login(ctx, &dto.LoginRequest{Email: email, Password: password})
	if err == nil {
		t.Fatal("expected login to fail for locked account even with correct password")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.HTTPStatus != 401 {
		t.Errorf("expected 401 Unauthorized for locked account, got %v", err)
	}
}

func TestFailedLoginProtection_SuccessfulLoginResetsCounter(t *testing.T) {
	mr, redisClient := setupTestRedis(t)

	email := "user.reset@example.com"
	password := "SecretPass123!"
	cred, err := createTestUserCredential(email, password)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	repo := newMockAuthRepository()
	_ = repo.CreateCredential(context.Background(), cred)

	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "test-secret", ExpiryMinutes: 60},
		Security: config.SecurityConfig{
			MaxLoginAttempts:    3,
			LoginAttemptWindow:  5 * time.Minute,
			AccountLockDuration: 10 * time.Minute,
		},
	}

	svc := NewAuthService(repo, cfg, nil, redisClient)
	ctx := context.Background()

	// 2 failed attempts
	_, _ = svc.Login(ctx, &dto.LoginRequest{Email: email, Password: "WrongPassword1"})
	_, _ = svc.Login(ctx, &dto.LoginRequest{Email: email, Password: "WrongPassword2"})

	failedKey := "auth:login:failed:" + email
	if val, _ := mr.Get(failedKey); val != "2" {
		t.Fatalf("expected counter 2, got %s", val)
	}

	// 3rd attempt: Correct password -> Successful Login
	resp, err := svc.Login(ctx, &dto.LoginRequest{Email: email, Password: password})
	if err != nil {
		t.Fatalf("expected successful login, got: %v", err)
	}
	if resp.AccessToken == "" {
		t.Fatal("expected access token in response")
	}

	// Redis counter and lock keys should be deleted
	if mr.Exists(failedKey) {
		t.Error("expected failed attempts counter to be cleared after successful login")
	}

	// Next failed login attempt should start count at 1 (not lock account)
	_, _ = svc.Login(ctx, &dto.LoginRequest{Email: email, Password: "WrongPasswordAgain"})
	if val, _ := mr.Get(failedKey); val != "1" {
		t.Errorf("expected counter to reset to 1 after new failed attempt, got %s", val)
	}
}

func TestFailedLoginProtection_AccountEnumerationPrevention(t *testing.T) {
	mr, redisClient := setupTestRedis(t)

	repo := newMockAuthRepository()
	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "test-secret", ExpiryMinutes: 60},
		Security: config.SecurityConfig{
			MaxLoginAttempts:    2,
			LoginAttemptWindow:  5 * time.Minute,
			AccountLockDuration: 10 * time.Minute,
		},
	}

	svc := NewAuthService(repo, cfg, nil, redisClient)
	ctx := context.Background()

	nonExistentEmail := "ghost.user@example.com"

	// Attempt 1 for non-existent user
	_, err1 := svc.Login(ctx, &dto.LoginRequest{Email: nonExistentEmail, Password: "SomePassword123!"})
	if err1 == nil {
		t.Fatal("expected error for non-existent user, got nil")
	}
	appErr1, ok1 := err1.(*appErrors.AppError)
	if !ok1 || appErr1.Message != "Invalid email or password" {
		t.Errorf("unexpected error message: %v", err1)
	}

	// Attempt 2 for non-existent user -> Triggers lock in Redis for non-existent email
	_, err2 := svc.Login(ctx, &dto.LoginRequest{Email: nonExistentEmail, Password: "SomePassword123!"})
	if err2 == nil {
		t.Fatal("expected error on attempt 2, got nil")
	}

	lockKey := "auth:login:locked:" + nonExistentEmail
	if !mr.Exists(lockKey) {
		t.Error("expected lock key in Redis for non-existent email after exceeding threshold")
	}

	// Attempt 3: Blocked by Redis lock check
	_, err3 := svc.Login(ctx, &dto.LoginRequest{Email: nonExistentEmail, Password: "SomePassword123!"})
	if err3 == nil {
		t.Fatal("expected error when locked, got nil")
	}
	appErr3, ok3 := err3.(*appErrors.AppError)
	if !ok3 || appErr3.Message != "Invalid email or password" {
		t.Errorf("expected generic unauthorized error message for locked non-existent account, got %v", err3)
	}
}

func TestFailedLoginProtection_RedisFailureFallback(t *testing.T) {
	email := "fallback.user@example.com"
	password := "ValidPass123!"
	cred, err := createTestUserCredential(email, password)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	repo := newMockAuthRepository()
	_ = repo.CreateCredential(context.Background(), cred)

	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "test-secret", ExpiryMinutes: 60},
		Security: config.SecurityConfig{
			MaxLoginAttempts:    2,
			LoginAttemptWindow:  5 * time.Minute,
			AccountLockDuration: 10 * time.Minute,
		},
	}

	svc := NewAuthService(repo, cfg, nil)
	ctx := context.Background()

	// Attempt 1: Wrong password -> DB updated with failed_login_count = 1
	_, err = svc.Login(ctx, &dto.LoginRequest{Email: email, Password: "WrongPassword1"})
	if err == nil {
		t.Fatal("expected error on attempt 1")
	}
	if repo.failedCountMap[cred.ID] != 1 {
		t.Errorf("expected DB failed count 1, got %d", repo.failedCountMap[cred.ID])
	}

	// Attempt 2: Wrong password -> Reaches threshold 2 -> DB locked_until set
	_, err = svc.Login(ctx, &dto.LoginRequest{Email: email, Password: "WrongPassword2"})
	if err == nil {
		t.Fatal("expected error on attempt 2")
	}
	if repo.failedCountMap[cred.ID] != 2 {
		t.Errorf("expected DB failed count 2, got %d", repo.failedCountMap[cred.ID])
	}
	if repo.lockedUntilMap[cred.ID] == nil || time.Now().After(*repo.lockedUntilMap[cred.ID]) {
		t.Errorf("expected DB locked_until to be set in the future, got %v", repo.lockedUntilMap[cred.ID])
	}

	// Attempt 3: Try logging in with CORRECT password -> Fails because DB is locked
	_, err = svc.Login(ctx, &dto.LoginRequest{Email: email, Password: password})
	if err == nil {
		t.Fatal("expected login to fail when account is locked in DB")
	}
}

func TestFailedLoginProtection_ConfigFromEnv(t *testing.T) {
	_ = os.Setenv("MAX_LOGIN_ATTEMPTS", "3")
	_ = os.Setenv("LOGIN_ATTEMPT_WINDOW", "10m")
	_ = os.Setenv("ACCOUNT_LOCK_DURATION", "20m")
	defer func() {
		_ = os.Unsetenv("MAX_LOGIN_ATTEMPTS")
		_ = os.Unsetenv("LOGIN_ATTEMPT_WINDOW")
		_ = os.Unsetenv("ACCOUNT_LOCK_DURATION")
	}()

	cfg := config.LoadEnv()
	if cfg.Security.MaxLoginAttempts != 3 {
		t.Errorf("expected MaxLoginAttempts 3, got %d", cfg.Security.MaxLoginAttempts)
	}
	if cfg.Security.LoginAttemptWindow != 10*time.Minute {
		t.Errorf("expected LoginAttemptWindow 10m, got %v", cfg.Security.LoginAttemptWindow)
	}
	if cfg.Security.AccountLockDuration != 20*time.Minute {
		t.Errorf("expected AccountLockDuration 20m, got %v", cfg.Security.AccountLockDuration)
	}
}

// ---------------------------------------------------------------------------
// Password Reset Flow Tests
// ---------------------------------------------------------------------------

type mockMailerWithCapture struct {
	lastToEmail string
	lastOTP     string
	resetEmails map[string]string
}

func newMockMailerWithCapture() *mockMailerWithCapture {
	return &mockMailerWithCapture{
		resetEmails: make(map[string]string),
	}
}

func (m *mockMailerWithCapture) SendVerificationEmail(ctx context.Context, toEmail, otp string) error {
	return nil
}

func (m *mockMailerWithCapture) SendPasswordResetEmail(ctx context.Context, toEmail, otp string) error {
	m.lastToEmail = toEmail
	m.lastOTP = otp
	m.resetEmails[toEmail] = otp
	return nil
}

func setupPasswordTestService() (AuthService, *mockAuthRepository, *mockMailerWithCapture, otp.PasswordResetStore, *config.Config) {
	mockRepo := newMockAuthRepository()
	mockMailer := newMockMailerWithCapture()
	resetStore := otp.NewMemoryPasswordResetStore()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-1234567890123456",
			ExpiryMinutes: 15,
		},
		Security: config.SecurityConfig{
			MaxLoginAttempts:             5,
			LoginAttemptWindow:           15 * time.Minute,
			AccountLockDuration:          15 * time.Minute,
			PasswordResetOTPTTLMinutes:   15,
			PasswordResetMaxRequests:     3,
			PasswordResetRequestWindow:   30 * time.Minute,
			PasswordResetMaxAttempts:     5,
			PasswordResetLockoutDuration: 1 * time.Hour,
		},
	}

	svc := NewAuthServiceWithMailer(mockRepo, cfg, mockMailer, nil, resetStore)
	return svc, mockRepo, mockMailer, resetStore, cfg
}

func TestForgotPassword_EmailMasking_UniformResponse(t *testing.T) {
	svc, mockRepo, mockMailer, _, _ := setupPasswordTestService()
	ctx := context.Background()

	existingEmail := "existing@example.com"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("Password123!"), bcrypt.DefaultCost)
	_ = mockRepo.CreateCredential(ctx, &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        uuid.Must(uuid.NewV7()),
		Email:         existingEmail,
		PasswordHash:  string(hashedPassword),
		Role:          model.RoleCustomer,
		EmailVerified: true,
		IsActive:      true,
	})

	expectedMessage := "If an account exists with this email address, a password reset code has been sent."

	// 1. Existing user
	resp1, err1 := svc.ForgotPassword(ctx, &dto.ForgotPasswordRequest{Email: existingEmail})
	if err1 != nil {
		t.Fatalf("unexpected error for existing email: %v", err1)
	}
	if !resp1.Success || resp1.Message != expectedMessage {
		t.Errorf("expected success=true and uniform message, got: %+v", resp1)
	}
	time.Sleep(10 * time.Millisecond)
	if mockMailer.lastToEmail != existingEmail || len(mockMailer.lastOTP) != 6 {
		t.Errorf("expected OTP sent to %s, got to=%s, otp=%s", existingEmail, mockMailer.lastToEmail, mockMailer.lastOTP)
	}

	// 2. Non-existent user
	nonExistentEmail := "nobody@example.com"
	mockMailer.lastToEmail = ""
	mockMailer.lastOTP = ""
	resp2, err2 := svc.ForgotPassword(ctx, &dto.ForgotPasswordRequest{Email: nonExistentEmail})
	if err2 != nil {
		t.Fatalf("unexpected error for non-existent email: %v", err2)
	}
	if !resp2.Success || resp2.Message != expectedMessage {
		t.Errorf("expected success=true and uniform message for non-existent email, got: %+v", resp2)
	}
	time.Sleep(10 * time.Millisecond)
	if mockMailer.lastToEmail == nonExistentEmail {
		t.Errorf("expected no email dispatched for non-existent user, but got: %s", mockMailer.lastToEmail)
	}
}

func TestForgotPassword_RateLimiting(t *testing.T) {
	svc, mockRepo, _, _, _ := setupPasswordTestService()
	ctx := context.Background()

	email := "ratelimit@example.com"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("Password123!"), bcrypt.DefaultCost)
	_ = mockRepo.CreateCredential(ctx, &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        uuid.Must(uuid.NewV7()),
		Email:         email,
		PasswordHash:  string(hashedPassword),
		Role:          model.RoleCustomer,
		EmailVerified: true,
		IsActive:      true,
	})

	for i := 1; i <= 3; i++ {
		resp, err := svc.ForgotPassword(ctx, &dto.ForgotPasswordRequest{Email: email})
		if err != nil || !resp.Success {
			t.Fatalf("expected request %d to succeed, got resp=%+v, err=%v", i, resp, err)
		}
	}

	_, err := svc.ForgotPassword(ctx, &dto.ForgotPasswordRequest{Email: email})
	if err == nil {
		t.Fatal("expected 4th request to be blocked by rate limit, but it succeeded")
	}
	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeTooManyRequests {
		t.Fatalf("expected CodeTooManyRequests, got: %v", err)
	}
}

func TestResetPasswordWithOtp_SuccessAndLogin(t *testing.T) {
	svc, mockRepo, mockMailer, _, _ := setupPasswordTestService()
	ctx := context.Background()

	email := "resetuser@example.com"
	oldPassword := "OldPassword123!"
	newPassword := "NewPassword123!"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(oldPassword), bcrypt.DefaultCost)
	userID := uuid.Must(uuid.NewV7())

	_ = mockRepo.CreateCredential(ctx, &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        userID,
		Email:         email,
		PasswordHash:  string(hashedPassword),
		Role:          model.RoleCustomer,
		EmailVerified: true,
		IsActive:      true,
	})

	mockRepo.refreshTokens = append(mockRepo.refreshTokens, &model.RefreshToken{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		TokenHash: "active-token-hash",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Revoked:   false,
	})

	_, err := svc.ForgotPassword(ctx, &dto.ForgotPasswordRequest{Email: email})
	if err != nil {
		t.Fatalf("forgot password failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	otpCode := mockMailer.lastOTP
	if otpCode == "" {
		t.Fatal("expected OTP code captured")
	}

	resetResp, err := svc.ResetPasswordWithOtp(ctx, &dto.ResetPasswordWithOtpRequest{
		Email:       email,
		OTP:         otpCode,
		NewPassword: newPassword,
	})
	if err != nil {
		t.Fatalf("reset password failed: %v", err)
	}
	if !resetResp.Success {
		t.Errorf("expected success true, got false")
	}

	for _, rt := range mockRepo.refreshTokens {
		if rt.UserID == userID && !rt.Revoked {
			t.Errorf("expected refresh token to be revoked")
		}
	}

	_, errOldLogin := svc.Login(ctx, &dto.LoginRequest{
		Email:    email,
		Password: oldPassword,
	})
	if errOldLogin == nil {
		t.Fatal("expected login with old password to fail")
	}

	loginResp, errNewLogin := svc.Login(ctx, &dto.LoginRequest{
		Email:    email,
		Password: newPassword,
	})
	if errNewLogin != nil {
		t.Fatalf("login with new password failed: %v", errNewLogin)
	}
	if loginResp.AccessToken == "" {
		t.Fatal("expected valid access token on login with new password")
	}
}

func TestResetPasswordWithOtp_SingleUseReuseFails(t *testing.T) {
	svc, mockRepo, mockMailer, _, _ := setupPasswordTestService()
	ctx := context.Background()

	email := "singleuse@example.com"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("Password123!"), bcrypt.DefaultCost)
	_ = mockRepo.CreateCredential(ctx, &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        uuid.Must(uuid.NewV7()),
		Email:         email,
		PasswordHash:  string(hashedPassword),
		Role:          model.RoleCustomer,
		EmailVerified: true,
		IsActive:      true,
	})

	_, _ = svc.ForgotPassword(ctx, &dto.ForgotPasswordRequest{Email: email})
	time.Sleep(10 * time.Millisecond)
	otpCode := mockMailer.lastOTP

	_, err := svc.ResetPasswordWithOtp(ctx, &dto.ResetPasswordWithOtpRequest{
		Email:       email,
		OTP:         otpCode,
		NewPassword: "FirstResetPass123!",
	})
	if err != nil {
		t.Fatalf("first reset failed: %v", err)
	}

	_, errReuse := svc.ResetPasswordWithOtp(ctx, &dto.ResetPasswordWithOtpRequest{
		Email:       email,
		OTP:         otpCode,
		NewPassword: "SecondResetPass123!",
	})
	if errReuse == nil {
		t.Fatal("expected reuse of OTP to fail, but it succeeded")
	}
}

func TestResetPasswordWithOtp_LockoutAfter5InvalidAttempts(t *testing.T) {
	svc, mockRepo, mockMailer, _, _ := setupPasswordTestService()
	ctx := context.Background()

	email := "lockout@example.com"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("Password123!"), bcrypt.DefaultCost)
	_ = mockRepo.CreateCredential(ctx, &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        uuid.Must(uuid.NewV7()),
		Email:         email,
		PasswordHash:  string(hashedPassword),
		Role:          model.RoleCustomer,
		EmailVerified: true,
		IsActive:      true,
	})

	_, _ = svc.ForgotPassword(ctx, &dto.ForgotPasswordRequest{Email: email})
	time.Sleep(10 * time.Millisecond)
	correctOTP := mockMailer.lastOTP

	for i := 1; i <= 4; i++ {
		_, err := svc.ResetPasswordWithOtp(ctx, &dto.ResetPasswordWithOtpRequest{
			Email:       email,
			OTP:         "000000",
			NewPassword: "NewValidPassword123!",
		})
		if err == nil {
			t.Fatalf("attempt %d with wrong OTP should fail", i)
		}
	}

	_, err5 := svc.ResetPasswordWithOtp(ctx, &dto.ResetPasswordWithOtpRequest{
		Email:       email,
		OTP:         "000000",
		NewPassword: "NewValidPassword123!",
	})
	if err5 == nil {
		t.Fatal("attempt 5 should fail")
	}
	appErr5, ok := err5.(*appErrors.AppError)
	if !ok || appErr5.Code != appErrors.CodeTooManyRequests {
		t.Fatalf("expected CodeTooManyRequests on 5th failure, got: %v", err5)
	}

	_, errCorrect := svc.ResetPasswordWithOtp(ctx, &dto.ResetPasswordWithOtpRequest{
		Email:       email,
		OTP:         correctOTP,
		NewPassword: "NewValidPassword123!",
	})
	if errCorrect == nil {
		t.Fatal("expected attempt with correct OTP to be blocked during lockout window")
	}
	appErrLock, ok := errCorrect.(*appErrors.AppError)
	if !ok || appErrLock.Code != appErrors.CodeTooManyRequests {
		t.Fatalf("expected CodeTooManyRequests during lockout, got: %v", errCorrect)
	}
}

func TestResetPasswordWithOtp_PasswordComplexity(t *testing.T) {
	svc, mockRepo, mockMailer, _, _ := setupPasswordTestService()
	ctx := context.Background()

	email := "complexity@example.com"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("Password123!"), bcrypt.DefaultCost)
	_ = mockRepo.CreateCredential(ctx, &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        uuid.Must(uuid.NewV7()),
		Email:         email,
		PasswordHash:  string(hashedPassword),
		Role:          model.RoleCustomer,
		EmailVerified: true,
		IsActive:      true,
	})

	_, _ = svc.ForgotPassword(ctx, &dto.ForgotPasswordRequest{Email: email})
	time.Sleep(10 * time.Millisecond)
	otpCode := mockMailer.lastOTP

	weakPasswords := []string{
		"short",
		"nouppercase123!",
		"NOLOWERCASE123!",
		"NoNumber!",
		"NoSpecialChar123",
	}

	for _, weak := range weakPasswords {
		_, err := svc.ResetPasswordWithOtp(ctx, &dto.ResetPasswordWithOtpRequest{
			Email:       email,
			OTP:         otpCode,
			NewPassword: weak,
		})
		if err == nil {
			t.Errorf("expected weak password '%s' to be rejected", weak)
		}
	}
}

func TestChangePassword_SuccessAndCredentialVerification(t *testing.T) {
	svc, mockRepo, _, _, _ := setupPasswordTestService()
	ctx := context.Background()

	email := "changepass@example.com"
	oldPassword := "OldPassword123!"
	newPassword := "NewPassword123!"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(oldPassword), bcrypt.DefaultCost)
	userID := uuid.Must(uuid.NewV7())

	_ = mockRepo.CreateCredential(ctx, &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        userID,
		Email:         email,
		PasswordHash:  string(hashedPassword),
		Role:          model.RoleCustomer,
		EmailVerified: true,
		IsActive:      true,
	})

	mockRepo.refreshTokens = append(mockRepo.refreshTokens, &model.RefreshToken{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		TokenHash: "active-token-change-pass",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Revoked:   false,
	})

	// 1. Calling changePassword with incorrect old password -> must fail with INVALID_CREDENTIALS
	_, errWrongOld := svc.ChangePassword(ctx, &dto.ChangePasswordRequest{
		UserID:      userID.String(),
		OldPassword: "WrongOldPassword123!",
		NewPassword: newPassword,
	})
	if errWrongOld == nil {
		t.Fatal("expected changePassword with wrong old password to fail")
	}
	appErrWrong, ok := errWrongOld.(*appErrors.AppError)
	if !ok || appErrWrong.Code != appErrors.CodeInvalidCredentials {
		t.Fatalf("expected CodeInvalidCredentials, got: %v", errWrongOld)
	}

	// 2. Calling with newPassword == oldPassword -> must fail
	_, errSame := svc.ChangePassword(ctx, &dto.ChangePasswordRequest{
		UserID:      userID.String(),
		OldPassword: oldPassword,
		NewPassword: oldPassword,
	})
	if errSame == nil {
		t.Fatal("expected changePassword with same new password to fail")
	}

	// 3. Calling with weak newPassword -> must fail
	_, errWeak := svc.ChangePassword(ctx, &dto.ChangePasswordRequest{
		UserID:      userID.String(),
		OldPassword: oldPassword,
		NewPassword: "weak",
	})
	if errWeak == nil {
		t.Fatal("expected changePassword with weak new password to fail")
	}

	// 4. Calling with correct old password and valid new password -> succeeds!
	resp, err := svc.ChangePassword(ctx, &dto.ChangePasswordRequest{
		UserID:      userID.String(),
		OldPassword: oldPassword,
		NewPassword: newPassword,
	})
	if err != nil {
		t.Fatalf("changePassword failed: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success true, got false")
	}

	for _, rt := range mockRepo.refreshTokens {
		if rt.UserID == userID && !rt.Revoked {
			t.Errorf("expected refresh token to be revoked")
		}
	}

	_, errOld := svc.Login(ctx, &dto.LoginRequest{Email: email, Password: oldPassword})
	if errOld == nil {
		t.Fatal("expected login with old password to fail")
	}

	loginResp, errNew := svc.Login(ctx, &dto.LoginRequest{Email: email, Password: newPassword})
	if errNew != nil {
		t.Fatalf("login with new password failed: %v", errNew)
	}
	if loginResp.AccessToken == "" {
		t.Fatal("expected valid access token on login with new password")
	}
}





