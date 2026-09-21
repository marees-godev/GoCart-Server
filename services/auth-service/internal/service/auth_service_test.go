package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/golang-jwt/jwt/v5"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type mockAuthRepository struct {
	byEmail           map[string]*model.AuthCredential
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
	m.byEmail[cred.Email] = cred
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
	svc := NewAuthService(mockRepo, cfg)
	return svc, mockRepo, cfg
}

func TestLogin_Success(t *testing.T) {
	svc, mockRepo, cfg := setupTestService()

	userID := uuid.New()
	credID := uuid.New()
	rawPassword := "StrongPassword123!"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)

	mockRepo.byEmail["user@example.com"] = &model.AuthCredential{
		ID:               credID,
		UserID:           userID,
		Email:            "user@example.com",
		PasswordHash:     string(hashedPassword),
		Role:             "customer",
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
	if claims.Role != "customer" {
		t.Errorf("expected role customer, got %s", claims.Role)
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
		ID:           uuid.New(),
		UserID:       uuid.New(),
		Email:        "user@example.com",
		PasswordHash: string(hashedPassword),
		Role:         "customer",
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
	inactiveID := uuid.New()
	mockRepo.byEmail["inactive@example.com"] = &model.AuthCredential{
		ID:           inactiveID,
		UserID:       uuid.New(),
		Email:        "inactive@example.com",
		PasswordHash: string(hashedPassword),
		IsActive:     false,
	}

	// Locked account
	lockedID := uuid.New()
	futureLock := time.Now().Add(10 * time.Minute)
	mockRepo.byEmail["locked@example.com"] = &model.AuthCredential{
		ID:           lockedID,
		UserID:       uuid.New(),
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

	credID := uuid.New()
	userID := uuid.New()
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
