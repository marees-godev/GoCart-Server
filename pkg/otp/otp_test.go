package otp_test

import (
	"testing"

	"github.com/marees-godev/GoCart-Server/pkg/otp"
)

func TestGenerateNumeric(t *testing.T) {
	code, err := otp.GenerateNumeric(6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("expected 6 digits, got: %s", code)
	}

	code4, err := otp.GenerateNumeric(4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(code4) != 4 {
		t.Fatalf("expected 4 digits, got: %s", code4)
	}
}

func TestGenerateRandomToken(t *testing.T) {
	token, err := otp.GenerateRandomToken(16)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(token) != 32 { // hex encoded
		t.Fatalf("expected 32 hex chars, got: %d", len(token))
	}
}

func TestHashAndVerify(t *testing.T) {
	token := "123456"
	hash := otp.HashToken(token)
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	if !otp.VerifyHash(token, hash) {
		t.Fatal("expected token to verify against its hash")
	}

	if otp.VerifyHash("wrong", hash) {
		t.Fatal("expected wrong token not to verify")
	}
}
