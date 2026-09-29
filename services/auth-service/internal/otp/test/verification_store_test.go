package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	pkgredis "github.com/marees-godev/GoCart-Server/pkg/redis"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/otp"
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
		t.Fatalf("failed to create redis client: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	return mr, client
}

// ---------------------------------------------------------------------------
// Memory Store Tests
// ---------------------------------------------------------------------------

func TestMemoryVerificationStore_SetAndGet(t *testing.T) {
	ctx := context.Background()
	store := otp.NewMemoryStore()

	email := "  Alice@Example.COM  "
	otpHash := "hashed-otp-123456"
	ttl := 5 * time.Minute

	if err := store.SetOTP(ctx, email, otpHash, ttl); err != nil {
		t.Fatalf("failed to set OTP: %v", err)
	}

	// Retrieve with trimmed / lowercase email
	got, err := store.GetOTP(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("failed to get OTP: %v", err)
	}
	if got != otpHash {
		t.Errorf("expected hash %q, got %q", otpHash, got)
	}
}

func TestMemoryVerificationStore_GetTTL(t *testing.T) {
	ctx := context.Background()
	store := otp.NewMemoryStore()

	email := "bob@example.com"
	otpHash := "hash-bob-654321"
	ttl := 10 * time.Minute

	if err := store.SetOTP(ctx, email, otpHash, ttl); err != nil {
		t.Fatalf("failed to set OTP: %v", err)
	}

	remaining, err := store.GetTTL(ctx, email)
	if err != nil {
		t.Fatalf("failed to get TTL: %v", err)
	}
	if remaining <= 0 || remaining > ttl {
		t.Errorf("expected remaining TTL between 0 and %v, got %v", ttl, remaining)
	}
}

func TestMemoryVerificationStore_Delete(t *testing.T) {
	ctx := context.Background()
	store := otp.NewMemoryStore()

	email := "carol@example.com"
	if err := store.SetOTP(ctx, email, "hash-carol", 5*time.Minute); err != nil {
		t.Fatalf("failed to set OTP: %v", err)
	}

	if err := store.DeleteOTP(ctx, email); err != nil {
		t.Fatalf("failed to delete OTP: %v", err)
	}

	_, err := store.GetOTP(ctx, email)
	if !errors.Is(err, otp.ErrOTPNotFound) {
		t.Errorf("expected ErrOTPNotFound after deletion, got %v", err)
	}
}

func TestMemoryVerificationStore_NotFound(t *testing.T) {
	ctx := context.Background()
	store := otp.NewMemoryStore()

	_, err := store.GetOTP(ctx, "nonexistent@example.com")
	if !errors.Is(err, otp.ErrOTPNotFound) {
		t.Errorf("expected ErrOTPNotFound, got %v", err)
	}

	_, err = store.GetTTL(ctx, "nonexistent@example.com")
	if !errors.Is(err, otp.ErrOTPNotFound) {
		t.Errorf("expected ErrOTPNotFound for TTL, got %v", err)
	}
}

func TestMemoryVerificationStore_Expiration(t *testing.T) {
	ctx := context.Background()
	store := otp.NewMemoryStore()

	email := "expiring@example.com"
	if err := store.SetOTP(ctx, email, "hash-fast-expiring", 10*time.Millisecond); err != nil {
		t.Fatalf("failed to set OTP: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	_, err := store.GetOTP(ctx, email)
	if !errors.Is(err, otp.ErrOTPNotFound) {
		t.Errorf("expected ErrOTPNotFound after expiry, got %v", err)
	}

	_, err = store.GetTTL(ctx, email)
	if !errors.Is(err, otp.ErrOTPNotFound) {
		t.Errorf("expected ErrOTPNotFound for TTL after expiry, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Redis Store Tests
// ---------------------------------------------------------------------------

func TestRedisVerificationStore_SetAndGet(t *testing.T) {
	mr, client := setupTestRedis(t)
	ctx := context.Background()
	store := otp.NewRedisStore(client)

	email := "redis.user@example.com"
	otpHash := "redis-hash-112233"
	ttl := 5 * time.Minute

	if err := store.SetOTP(ctx, email, otpHash, ttl); err != nil {
		t.Fatalf("failed to set OTP in redis: %v", err)
	}

	// Verify key format in redis
	expectedKey := "auth:otp:email:redis.user@example.com"
	if !mr.Exists(expectedKey) {
		t.Errorf("expected redis key %q to exist", expectedKey)
	}

	got, err := store.GetOTP(ctx, email)
	if err != nil {
		t.Fatalf("failed to get OTP from redis: %v", err)
	}
	if got != otpHash {
		t.Errorf("expected hash %q, got %q", otpHash, got)
	}
}

func TestRedisVerificationStore_GetTTL(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := context.Background()
	store := otp.NewRedisStore(client)

	email := "redis.ttl@example.com"
	ttl := 10 * time.Minute

	if err := store.SetOTP(ctx, email, "hash-ttl", ttl); err != nil {
		t.Fatalf("failed to set OTP: %v", err)
	}

	remaining, err := store.GetTTL(ctx, email)
	if err != nil {
		t.Fatalf("failed to get TTL: %v", err)
	}
	if remaining <= 0 || remaining > ttl {
		t.Errorf("expected remaining TTL between 0 and %v, got %v", ttl, remaining)
	}
}

func TestRedisVerificationStore_Delete(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := context.Background()
	store := otp.NewRedisStore(client)

	email := "redis.del@example.com"
	if err := store.SetOTP(ctx, email, "hash-del", 5*time.Minute); err != nil {
		t.Fatalf("failed to set OTP: %v", err)
	}

	if err := store.DeleteOTP(ctx, email); err != nil {
		t.Fatalf("failed to delete OTP: %v", err)
	}

	_, err := store.GetOTP(ctx, email)
	if !errors.Is(err, otp.ErrOTPNotFound) {
		t.Errorf("expected ErrOTPNotFound after redis delete, got %v", err)
	}
}

func TestRedisVerificationStore_NotFound(t *testing.T) {
	_, client := setupTestRedis(t)
	ctx := context.Background()
	store := otp.NewRedisStore(client)

	_, err := store.GetOTP(ctx, "nonexistent@example.com")
	if !errors.Is(err, otp.ErrOTPNotFound) {
		t.Errorf("expected ErrOTPNotFound, got %v", err)
	}
}
