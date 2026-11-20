package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateIndianPincode(t *testing.T) {
	tests := []struct {
		name      string
		pincode   string
		expectErr bool
		errMsg    string
	}{
		{
			name:      "empty pincode",
			pincode:   "",
			expectErr: true,
			errMsg:    "PIN code is required",
		},
		{
			name:      "contains alphabetic characters",
			pincode:   "11000A",
			expectErr: true,
			errMsg:    "must contain only numbers",
		},
		{
			name:      "contains special characters",
			pincode:   "110-01",
			expectErr: true,
			errMsg:    "must contain only numbers",
		},
		{
			name:      "too short (5 digits)",
			pincode:   "11000",
			expectErr: true,
			errMsg:    "must be exactly 6 digits",
		},
		{
			name:      "too long (7 digits)",
			pincode:   "1100001",
			expectErr: true,
			errMsg:    "must be exactly 6 digits",
		},
		{
			name:      "starts with 0",
			pincode:   "012345",
			expectErr: true,
			errMsg:    "cannot start with 0",
		},
		{
			name:      "all identical digits - 111111",
			pincode:   "111111",
			expectErr: true,
			errMsg:    "repeating digits are not allowed",
		},
		{
			name:      "all identical digits - 222222",
			pincode:   "222222",
			expectErr: true,
			errMsg:    "repeating digits are not allowed",
		},
		{
			name:      "all identical digits - 999999",
			pincode:   "999999",
			expectErr: true,
			errMsg:    "repeating digits are not allowed",
		},
		{
			name:      "sequential ascending - 123456",
			pincode:   "123456",
			expectErr: true,
			errMsg:    "sequential patterns are not allowed",
		},
		{
			name:      "sequential descending - 654321",
			pincode:   "654321",
			expectErr: true,
			errMsg:    "sequential patterns are not allowed",
		},
		{
			name:      "dummy repetitive pattern - 121212",
			pincode:   "121212",
			expectErr: true,
			errMsg:    "dummy pattern not allowed",
		},
		{
			name:      "invalid circle prefix - 101234",
			pincode:   "101234",
			expectErr: true,
			errMsg:    "invalid postal circle prefix",
		},
		{
			name:      "invalid circle prefix - 291234",
			pincode:   "291234",
			expectErr: true,
			errMsg:    "invalid postal circle prefix",
		},
		{
			name:      "Delhi invalid 3rd digit - 111001",
			pincode:   "111001",
			expectErr: true,
			errMsg:    "Delhi PIN codes must be in the range 110001 - 110099",
		},
		{
			name:      "Delhi invalid 4th digit - 110101",
			pincode:   "110101",
			expectErr: true,
			errMsg:    "Delhi PIN codes must be in the range 110001 - 110099",
		},
		{
			name:      "Delhi invalid boundary - 110000",
			pincode:   "110000",
			expectErr: true,
			errMsg:    "Delhi PIN codes must be in the range 110001 - 110099",
		},
		{
			name:      "valid Delhi pincode - 110001 (Connaught Place / GPO)",
			pincode:   "110001",
			expectErr: false,
		},
		{
			name:      "valid Delhi pincode - 110092",
			pincode:   "110092",
			expectErr: false,
		},
		{
			name:      "valid Tamil Nadu pincode - 600001 (Chennai GPO)",
			pincode:   "600001",
			expectErr: false,
		},
		{
			name:      "valid Tamil Nadu pincode - 623517",
			pincode:   "623517",
			expectErr: false,
		},
		{
			name:      "valid Karnataka pincode - 560001 (Bangalore GPO)",
			pincode:   "560001",
			expectErr: false,
		},
		{
			name:      "valid Maharashtra pincode - 400001 (Mumbai GPO)",
			pincode:   "400001",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIndianPincode(tt.pincode)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePostalCode(t *testing.T) {
	t.Run("India default", func(t *testing.T) {
		assert.NoError(t, ValidatePostalCode("600001", "IN"))
		assert.NoError(t, ValidatePostalCode("110001", "India"))
		assert.Error(t, ValidatePostalCode("111111", "IN"))
		assert.Error(t, ValidatePostalCode("110101", "IN"))
	})

	t.Run("International", func(t *testing.T) {
		assert.NoError(t, ValidatePostalCode("90210", "US"))
		assert.Error(t, ValidatePostalCode("12", "US"))
		assert.Error(t, ValidatePostalCode("11111", "US"))
	})
}
