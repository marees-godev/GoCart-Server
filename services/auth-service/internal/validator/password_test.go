package validator

import (
	"testing"
)

func TestValidatePasswordStrength(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{
			name:     "valid complex password",
			password: "ValidPassword123!",
			wantErr:  nil,
		},
		{
			name:     "too short (< 8 chars)",
			password: "Ab1!",
			wantErr:  ErrPasswordTooShort,
		},
		{
			name:     "missing uppercase",
			password: "password123!",
			wantErr:  ErrPasswordMissingUpper,
		},
		{
			name:     "missing lowercase",
			password: "PASSWORD123!",
			wantErr:  ErrPasswordMissingLower,
		},
		{
			name:     "missing digit",
			password: "Password!",
			wantErr:  ErrPasswordMissingDigit,
		},
		{
			name:     "missing special character",
			password: "Password123",
			wantErr:  ErrPasswordMissingSpecial,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePasswordStrength(tt.password)
			if tt.wantErr == nil && err != nil {
				t.Fatalf("expected nil error, got: %v", err)
			}
			if tt.wantErr != nil && err != tt.wantErr {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}
