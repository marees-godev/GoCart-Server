package otp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// GenerateNumeric returns a cryptographically secure random numeric OTP string of given length (e.g. 6 digits).
func GenerateNumeric(digits int) (string, error) {
	if digits <= 0 {
		digits = 6
	}
	min := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits-1)), nil)
	max := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	diff := new(big.Int).Sub(max, min)

	n, err := rand.Int(rand.Reader, diff)
	if err != nil {
		return "", err
	}
	val := new(big.Int).Add(n, min)
	format := fmt.Sprintf("%%0%dd", digits)
	return fmt.Sprintf(format, val.Int64()), nil
}

// GenerateRandomToken generates a cryptographically secure hex-encoded random string of nBytes.
func GenerateRandomToken(nBytes int) (string, error) {
	if nBytes <= 0 {
		nBytes = 32
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken computes a SHA-256 hexadecimal digest of the given token or OTP string.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}

// VerifyHash compares a raw token/OTP against an expected SHA-256 hexadecimal hash.
func VerifyHash(token, expectedHash string) bool {
	return HashToken(token) == expectedHash
}
