package security_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

// ---------------------------------------------------------------------------
// Security Tests for Auth & Authorization Controls
// ---------------------------------------------------------------------------

func TestSecurity_SensitiveDataNotExposed(t *testing.T) {
	// Simulate an error or API response struct
	appErr := appErrors.Unauthorized("invalid credentials provided")

	// Verify error output does not leak internal secrets, passwords, or hashes
	errStr := appErr.Error()
	sensitiveKeywords := []string{
		"password",
		"hash",
		"secret",
		"private_key",
		"token_hash",
		"DATABASE_URL",
	}

	for _, kw := range sensitiveKeywords {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(kw)) && !strings.Contains(errStr, "invalid credentials") {
			t.Errorf("SECURITY RISK: AppError leaks sensitive keyword '%s': %s", kw, errStr)
		}
	}

	// Verify JSON marshaling of app errors does not expose raw error stack trace to clients
	jsonBytes, err := json.Marshal(appErr)
	if err != nil {
		t.Fatalf("failed to marshal app error: %v", err)
	}

	jsonMap := make(map[string]interface{})
	if err := json.Unmarshal(jsonBytes, &jsonMap); err == nil {
		if _, hasStack := jsonMap["stack_trace"]; hasStack {
			t.Error("SECURITY RISK: JSON response contains internal stack_trace")
		}
		if _, hasRawErr := jsonMap["raw_error"]; hasRawErr {
			t.Error("SECURITY RISK: JSON response contains internal raw_error")
		}
	}
}

func TestSecurity_JWTAlgorithmNoneRejection(t *testing.T) {
	// Attempt to create token with "none" signing algorithm
	claims := auth.UserClaims{
		UserID: "attacker-id",
		Role:   "ADMIN",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}

	// Unsigned "none" token
	unsignedToken := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, err := unsignedToken.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to create unsigned token: %v", err)
	}

	// Validation with secret must reject "none" algorithm
	_, err = auth.ValidateToken(tokenString, "gocart-secret-key-32bytes")
	if err == nil {
		t.Fatal("CRITICAL SECURITY FAILURE: Token signed with 'none' algorithm was accepted!")
	}
}

func TestSecurity_RolePrivilegeEscalationPrevention(t *testing.T) {
	secret := "gocart-priv-escalation-test-secret"

	// Create valid CUSTOMER token
	custUser := auth.UserContext{
		UserID: "normal-user-123",
		Role:   auth.RoleCustomer,
	}

	tok, err := auth.GenerateToken(custUser, secret, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	validated, err := auth.ValidateToken(tok, secret)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}

	// Role cannot be elevated dynamically by consumer
	if validated.Role == auth.RoleAdmin || validated.Role == auth.RoleMerchant {
		t.Fatalf("CRITICAL SECURITY FAILURE: Role privilege escalation detected! User role: %s", validated.Role)
	}
}
