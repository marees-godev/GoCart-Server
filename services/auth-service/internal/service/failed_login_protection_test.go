package service

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofrs/uuid/v5"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	pkgredis "github.com/marees-godev/GoCart-Server/pkg/redis"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
	"golang.org/x/crypto/bcrypt"
)

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
	// Service initialized without Redis client (redisClient == nil)
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

	// No Redis client passed -> nil redis client
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
