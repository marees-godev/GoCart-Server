package auth

import (
	"context"
	"testing"
	"time"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func TestGenerateAndValidateToken_Success(t *testing.T) {
	secret := "test-secret-key"
	user := UserContext{
		UserID: "usr-123",
		Role:   "CUSTOMER",
		Email:  "test@example.com",
	}

	tokenStr, err := GenerateToken(user, secret, 1*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	validated, err := ValidateToken(tokenStr, secret)
	if err != nil {
		t.Fatalf("unexpected error validating token: %v", err)
	}

	if validated.UserID != user.UserID {
		t.Errorf("expected UserID %s, got %s", user.UserID, validated.UserID)
	}
	if validated.Role != user.Role {
		t.Errorf("expected Role %s, got %s", user.Role, validated.Role)
	}
	if validated.Email != user.Email {
		t.Errorf("expected Email %s, got %s", user.Email, validated.Email)
	}
}

func TestValidateToken_WithBearerPrefix(t *testing.T) {
	secret := "test-secret-key"
	user := UserContext{
		UserID: "usr-456",
		Role:   "ADMIN",
	}

	tokenStr, err := GenerateToken(user, secret, 1*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	validated, err := ValidateToken("Bearer "+tokenStr, secret)
	if err != nil {
		t.Fatalf("unexpected error validating token with Bearer prefix: %v", err)
	}

	if validated.UserID != user.UserID || validated.Role != user.Role {
		t.Errorf("validated user context mismatched")
	}
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	secret := "test-secret-key"
	user := UserContext{
		UserID: "usr-789",
		Role:   "CUSTOMER",
	}

	tokenStr, err := GenerateToken(user, secret, -1*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error generating expired token: %v", err)
	}

	_, err = ValidateToken(tokenStr, secret)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeUnauthorized {
		t.Errorf("expected UNAUTHORIZED code, got %s", appErr.Code)
	}
}

func TestValidateToken_InvalidSignature(t *testing.T) {
	user := UserContext{
		UserID: "usr-123",
		Role:   "CUSTOMER",
	}

	tokenStr, _ := GenerateToken(user, "secret-1", 1*time.Hour)
	_, err := ValidateToken(tokenStr, "secret-2")

	if err == nil {
		t.Fatal("expected error for wrong secret, got nil")
	}
}

func TestContextPropagation(t *testing.T) {
	user := &UserContext{
		UserID: "usr-999",
		Role:   "MERCHANT",
		Email:  "merchant@example.com",
	}

	ctx := WithUser(context.Background(), user)
	retrieved, ok := UserFromContext(ctx)
	if !ok || retrieved == nil {
		t.Fatalf("failed to retrieve user from context")
	}

	if retrieved.UserID != user.UserID || retrieved.Role != user.Role {
		t.Errorf("retrieved user mismatch")
	}
}
