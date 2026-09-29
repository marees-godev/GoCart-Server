package validator

import (
	"errors"
	"strings"
	"unicode"
)

var (
	ErrPasswordTooShort        = errors.New("password must be at least 8 characters long")
	ErrPasswordTooLong         = errors.New("password must not exceed 128 characters")
	ErrPasswordMissingUpper    = errors.New("password must contain at least one uppercase letter")
	ErrPasswordMissingLower    = errors.New("password must contain at least one lowercase letter")
	ErrPasswordMissingDigit    = errors.New("password must contain at least one number")
	ErrPasswordMissingSpecial  = errors.New("password must contain at least one special character")
	ErrPasswordSameAsOld       = errors.New("new password must be distinct from current password")
)

// ValidatePasswordStrength validates the password against security complexity rules.
func ValidatePasswordStrength(password string) error {
	password = strings.TrimSpace(password)
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	if len(password) > 128 {
		return ErrPasswordTooLong
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return ErrPasswordMissingUpper
	}
	if !hasLower {
		return ErrPasswordMissingLower
	}
	if !hasDigit {
		return ErrPasswordMissingDigit
	}
	if !hasSpecial {
		return ErrPasswordMissingSpecial
	}

	return nil
}
