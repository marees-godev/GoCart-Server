package dto

import (
	"testing"
)

func TestValidatePostalCode_AllSituations(t *testing.T) {
	tests := []struct {
		name       string
		postalCode string
		country    string
		wantErr    bool
		errSubstr  string
	}{
		// Alpha / Alphanumeric characters
		{
			name:       "Alphanumeric with letters like 905u75686588",
			postalCode: "905u75686588",
			country:    "India",
			wantErr:    true,
			errSubstr:  "must contain only numbers",
		},
		{
			name:       "Pure alphabets",
			postalCode: "ABCDEF",
			country:    "India",
			wantErr:    true,
			errSubstr:  "must contain only numbers",
		},
		{
			name:       "Special characters",
			postalCode: "!@#$%^",
			country:    "India",
			wantErr:    true,
			errSubstr:  "must contain only numbers",
		},
		{
			name:       "Numbers with spaces",
			postalCode: "623 517",
			country:    "India",
			wantErr:    true,
			errSubstr:  "must contain only numbers",
		},
		{
			name:       "Numbers with dashes",
			postalCode: "623-517",
			country:    "India",
			wantErr:    true,
			errSubstr:  "must contain only numbers",
		},
		// Empty / whitespace
		{
			name:       "Empty postal code",
			postalCode: "",
			country:    "India",
			wantErr:    true,
			errSubstr:  "postal_code is required",
		},
		{
			name:       "Whitespace only postal code",
			postalCode: "    ",
			country:    "India",
			wantErr:    true,
			errSubstr:  "postal_code is required",
		},
		// Length boundaries
		{
			name:       "Too short (less than 4 digits)",
			postalCode: "123",
			country:    "Other",
			wantErr:    true,
			errSubstr:  "must be at least 4 digits",
		},
		{
			name:       "Too long (more than 10 digits)",
			postalCode: "12345678901",
			country:    "Other",
			wantErr:    true,
			errSubstr:  "must not exceed 10 digits",
		},
		// Indian PIN code situations
		{
			name:       "Indian PIN code starts with 0",
			postalCode: "012345",
			country:    "India",
			wantErr:    true,
			errSubstr:  "cannot start with 0",
		},
		{
			name:       "Indian PIN code 5 digits",
			postalCode: "62351",
			country:    "India",
			wantErr:    true,
			errSubstr:  "must be exactly 6 digits",
		},
		{
			name:       "Indian PIN code 7 digits",
			postalCode: "6235177",
			country:    "India",
			wantErr:    true,
			errSubstr:  "must be exactly 6 digits",
		},
		{
			name:       "Indian PIN code all same digits 111111",
			postalCode: "111111",
			country:    "India",
			wantErr:    true,
			errSubstr:  "repeating digits are not allowed",
		},
		{
			name:       "Indian PIN code Delhi out of range 110101",
			postalCode: "110101",
			country:    "India",
			wantErr:    true,
			errSubstr:  "Delhi PIN codes must be in the range 110001 - 110099",
		},
		{
			name:       "Indian PIN code valid 623517 with country India",
			postalCode: "623517",
			country:    "India",
			wantErr:    false,
		},
		{
			name:       "Indian PIN code valid 110001 with country IN",
			postalCode: "110001",
			country:    "IN",
			wantErr:    false,
		},
		{
			name:       "Indian PIN code valid 560001 with country IND",
			postalCode: "560001",
			country:    "IND",
			wantErr:    false,
		},
		// US ZIP code situations
		{
			name:       "US ZIP code 3 digits",
			postalCode: "123",
			country:    "United States",
			wantErr:    true,
			errSubstr:  "must be at least 4 digits",
		},
		{
			name:       "US ZIP code 11 digits",
			postalCode: "12345678901",
			country:    "United States",
			wantErr:    true,
			errSubstr:  "must not exceed 10 digits",
		},
		{
			name:       "US ZIP code valid 97477",
			postalCode: "97477",
			country:    "United States",
			wantErr:    false,
		},
		{
			name:       "US ZIP code valid 623517",
			postalCode: "623517",
			country:    "United States",
			wantErr:    false,
		},
		// General / other countries
		{
			name:       "Other country valid 4 digits",
			postalCode: "1234",
			country:    "Australia",
			wantErr:    false,
		},
		{
			name:       "Other country valid 8 digits",
			postalCode: "12345678",
			country:    "Japan",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePostalCode(tt.postalCode, tt.country)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePostalCode(%q, %q) error = %v, wantErr %v", tt.postalCode, tt.country, err, tt.wantErr)
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || len(err.Error()) == 0 {
					t.Fatalf("expected error containing %q, got nil", tt.errSubstr)
				}
			}
		})
	}
}
