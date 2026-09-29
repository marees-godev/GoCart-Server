package test

import (
	"context"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/otp"
)

// ---------------------------------------------------------------------------
// Memory Store Password Reset Tests
// ---------------------------------------------------------------------------

func TestMemoryPasswordResetStore(t *testing.T) {
	ctx := context.Background()
	store := otp.NewMemoryPasswordResetStore()
	email := "user@example.com"

	// 1. Rate limiting
	for i := 0; i < 3; i++ {
		allowed, err := store.RecordRequest(ctx, email, 3, 30*time.Minute)
		if err != nil || !allowed {
			t.Fatalf("expected request %d to be allowed, got allowed=%v, err=%v", i+1, allowed, err)
		}
	}
	// 4th request must be rejected
	allowed, err := store.RecordRequest(ctx, email, 3, 30*time.Minute)
	if err != nil || allowed {
		t.Fatalf("expected 4th request to be blocked, got allowed=%v", allowed)
	}

	// 2. Set and Get OTP
	err = store.SetResetOTP(ctx, email, "hash123", "salt456", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to set reset OTP: %v", err)
	}

	data, err := store.GetResetOTP(ctx, email)
	if err != nil {
		t.Fatalf("failed to get reset OTP: %v", err)
	}
	if data.Hash != "hash123" || data.Salt != "salt456" {
		t.Fatalf("unexpected data: %+v", data)
	}

	// 3. Increment attempts
	attempts, err := store.IncrementAttempts(ctx, email)
	if err != nil || attempts != 1 {
		t.Fatalf("expected attempts=1, got %d, err=%v", attempts, err)
	}

	// 4. Lockout
	err = store.LockReset(ctx, email, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to lock reset: %v", err)
	}

	isLocked, remaining, err := store.IsLocked(ctx, email)
	if err != nil || !isLocked || remaining <= 0 {
		t.Fatalf("expected locked, got isLocked=%v, remaining=%v", isLocked, remaining)
	}

	// OTP should be purged on lockout
	_, err = store.GetResetOTP(ctx, email)
	if err != otp.ErrResetOTPNotFound {
		t.Fatalf("expected ErrResetOTPNotFound after lockout, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Redis Store Password Reset Tests
// ---------------------------------------------------------------------------

func TestRedisPasswordResetStore(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := context.Background()
	store := otp.NewRedisPasswordResetStore(client)
	email := "redis.reset@example.com"

	// 1. Rate limiting
	for i := 0; i < 3; i++ {
		allowed, err := store.RecordRequest(ctx, email, 3, 30*time.Minute)
		if err != nil || !allowed {
			t.Fatalf("expected request %d to be allowed, got allowed=%v, err=%v", i+1, allowed, err)
		}
	}
	allowed, err := store.RecordRequest(ctx, email, 3, 30*time.Minute)
	if err != nil || allowed {
		t.Fatalf("expected 4th request to be blocked, got allowed=%v", allowed)
	}

	// 2. Set and Get OTP
	err = store.SetResetOTP(ctx, email, "redis-hash-abc", "redis-salt-xyz", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to set reset OTP in redis: %v", err)
	}

	data, err := store.GetResetOTP(ctx, email)
	if err != nil {
		t.Fatalf("failed to get reset OTP from redis: %v", err)
	}
	if data.Hash != "redis-hash-abc" || data.Salt != "redis-salt-xyz" {
		t.Fatalf("unexpected data: %+v", data)
	}

	// 3. Increment attempts
	attempts, err := store.IncrementAttempts(ctx, email)
	if err != nil || attempts != 1 {
		t.Fatalf("expected attempts=1, got %d, err=%v", attempts, err)
	}

	// 4. Lockout
	err = store.LockReset(ctx, email, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to lock reset: %v", err)
	}

	isLocked, remaining, err := store.IsLocked(ctx, email)
	if err != nil || !isLocked || remaining <= 0 {
		t.Fatalf("expected locked, got isLocked=%v, remaining=%v", isLocked, remaining)
	}

	// OTP should be purged on lockout
	_, err = store.GetResetOTP(ctx, email)
	if err != otp.ErrResetOTPNotFound {
		t.Fatalf("expected ErrResetOTPNotFound after lockout, got %v", err)
	}

	// 5. Delete reset OTP
	_ = store.SetResetOTP(ctx, email, "newhash", "newsalt", 10*time.Minute)
	if err := store.DeleteResetOTP(ctx, email); err != nil {
		t.Fatalf("failed to delete reset OTP: %v", err)
	}
	_, err = store.GetResetOTP(ctx, email)
	if err != otp.ErrResetOTPNotFound {
		t.Fatalf("expected ErrResetOTPNotFound after deletion, got %v", err)
	}
}
