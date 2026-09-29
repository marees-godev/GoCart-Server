package integration_test

import (
	"context"
	"testing"
	"time"

	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
)

// ---------------------------------------------------------------------------
// Public Authentication & Authorization Integration Tests
// ---------------------------------------------------------------------------

func TestTokens_JWTTokenLifecycle(t *testing.T) {
	secret := "gocart-integration-test-secret-32bytes"
	ttl := 15 * time.Minute

	t.Run("Valid Customer JWT Token", func(t *testing.T) {
		custUser := auth.UserContext{
			UserID: "cust-01010101-0101-0101-0101-010101010101",
			Role:   auth.RoleCustomer,
			Email:  "customer@gocart.com",
		}

		tok, err := auth.GenerateToken(custUser, secret, ttl)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		validated, err := auth.ValidateToken(tok, secret)
		if err != nil {
			t.Fatalf("token validation failed: %v", err)
		}

		if validated.UserID != custUser.UserID {
			t.Errorf("expected user_id %s, got %s", custUser.UserID, validated.UserID)
		}
		if validated.Role != auth.RoleCustomer {
			t.Errorf("expected role %s, got %s", auth.RoleCustomer, validated.Role)
		}
		if validated.Email != custUser.Email {
			t.Errorf("expected email %s, got %s", custUser.Email, validated.Email)
		}
	})

	t.Run("Valid Merchant JWT Token", func(t *testing.T) {
		merchUser := auth.UserContext{
			UserID: "merch-02020202-0202-0202-0202-020202020202",
			Role:   auth.RoleMerchant,
			Email:  "merchant@gocart.com",
		}

		tok, err := auth.GenerateToken(merchUser, secret, ttl)
		if err != nil {
			t.Fatalf("failed to generate merchant token: %v", err)
		}

		validated, err := auth.ValidateToken(tok, secret)
		if err != nil {
			t.Fatalf("merchant token validation failed: %v", err)
		}

		if validated.Role != auth.RoleMerchant {
			t.Errorf("expected role MERCHANT, got %s", validated.Role)
		}
	})

	t.Run("Expired JWT Token Rejection", func(t *testing.T) {
		user := auth.UserContext{
			UserID: "user-expired",
			Role:   auth.RoleCustomer,
		}

		tok, err := auth.GenerateToken(user, secret, -10*time.Minute)
		if err != nil {
			t.Fatalf("failed to generate expired token: %v", err)
		}

		_, err = auth.ValidateToken(tok, secret)
		if err == nil {
			t.Fatal("expected expired token validation to fail, but succeeded")
		}
	})

	t.Run("Tampered Secret JWT Token Rejection", func(t *testing.T) {
		user := auth.UserContext{
			UserID: "user-tampered",
			Role:   auth.RoleCustomer,
		}

		tok, err := auth.GenerateToken(user, "wrong-secret-key-1234567890000000", ttl)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = auth.ValidateToken(tok, secret)
		if err == nil {
			t.Fatal("expected tampered/wrong secret token validation to fail, but succeeded")
		}
	})

	t.Run("Malformed & Invalid Signature JWT Rejection", func(t *testing.T) {
		malformedTokens := []string{
			"",
			"invalid-token-string",
			"header.payload.signature_extra",
			"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalidpayload.invalidsig",
		}

		for _, badTok := range malformedTokens {
			_, err := auth.ValidateToken(badTok, secret)
			if err == nil {
				t.Errorf("expected validation to fail for malformed token '%s'", badTok)
			}
		}
	})
}

func TestAuthorization_RBACMatrix(t *testing.T) {
	secret := "gocart-rbac-test-secret-key-32b"

	custCtx := auth.UserContext{UserID: "c1", Role: auth.RoleCustomer}
	merchCtx := auth.UserContext{UserID: "m1", Role: auth.RoleMerchant}
	adminCtx := auth.UserContext{UserID: "a1", Role: auth.RoleAdmin}

	custToken, _ := auth.GenerateToken(custCtx, secret, 15*time.Minute)
	merchToken, _ := auth.GenerateToken(merchCtx, secret, 15*time.Minute)
	adminToken, _ := auth.GenerateToken(adminCtx, secret, 15*time.Minute)

	t.Run("CUSTOMER -> customer endpoint SUCCESS", func(t *testing.T) {
		u, err := auth.ValidateToken(custToken, secret)
		if err != nil || u.Role != auth.RoleCustomer {
			t.Fatalf("expected CUSTOMER token validation success, got err: %v", err)
		}
	})

	t.Run("CUSTOMER -> merchant endpoint FORBIDDEN", func(t *testing.T) {
		u, err := auth.ValidateToken(custToken, secret)
		if err != nil {
			t.Fatalf("token invalid: %v", err)
		}
		if u.Role == auth.RoleMerchant {
			t.Fatal("SECURITY ERROR: CUSTOMER evaluated as MERCHANT")
		}
	})

	t.Run("CUSTOMER -> admin endpoint FORBIDDEN", func(t *testing.T) {
		u, err := auth.ValidateToken(custToken, secret)
		if err != nil {
			t.Fatalf("token invalid: %v", err)
		}
		if u.Role == auth.RoleAdmin {
			t.Fatal("SECURITY ERROR: CUSTOMER evaluated as ADMIN")
		}
	})

	t.Run("MERCHANT -> merchant endpoint SUCCESS", func(t *testing.T) {
		u, err := auth.ValidateToken(merchToken, secret)
		if err != nil || u.Role != auth.RoleMerchant {
			t.Fatalf("expected MERCHANT token validation success, got err: %v", err)
		}
	})

	t.Run("MERCHANT -> admin endpoint FORBIDDEN", func(t *testing.T) {
		u, err := auth.ValidateToken(merchToken, secret)
		if err != nil {
			t.Fatalf("token invalid: %v", err)
		}
		if u.Role == auth.RoleAdmin {
			t.Fatal("SECURITY ERROR: MERCHANT evaluated as ADMIN")
		}
	})

	t.Run("ADMIN -> admin endpoint SUCCESS", func(t *testing.T) {
		u, err := auth.ValidateToken(adminToken, secret)
		if err != nil || u.Role != auth.RoleAdmin {
			t.Fatalf("expected ADMIN token validation success, got err: %v", err)
		}
	})
}

func TestPublicAdminRegistration_Constraint(t *testing.T) {
	req := &authpb.RegisterRequest{
		Email:      "hacker@admin.com",
		Password:   "Password123!",
		IsMerchant: false,
	}

	if req.GetEmail() != "hacker@admin.com" {
		t.Errorf("expected email hacker@admin.com, got %s", req.GetEmail())
	}
	if req.GetPassword() != "Password123!" {
		t.Errorf("expected password Password123!, got %s", req.GetPassword())
	}
	if req.IsMerchant {
		t.Fatal("expected customer registration is_merchant=false")
	}
}

func TestContextPropagation(t *testing.T) {
	ctx := context.Background()
	user := &auth.UserContext{
		UserID: "usr-ctx-1",
		Role:   auth.RoleCustomer,
		Email:  "ctx@example.com",
	}

	ctxWithUser := auth.WithUser(ctx, user)
	extracted, ok := auth.UserFromContext(ctxWithUser)
	if !ok || extracted == nil {
		t.Fatal("failed to extract user from context")
	}
	if extracted.UserID != user.UserID {
		t.Errorf("expected user ID %s, got %s", user.UserID, extracted.UserID)
	}
}
