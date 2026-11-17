package push

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const defaultVAPIDTTL = 12 * time.Hour

func DecodeURLSafeBase64(input string) ([]byte, error) {
	cleaned := strings.TrimSpace(input)
	cleaned = strings.ReplaceAll(cleaned, "+", "-")
	cleaned = strings.ReplaceAll(cleaned, "/", "_")
	cleaned = strings.TrimRight(cleaned, "=")
	if cleaned == "" {
		return nil, errors.New("empty base64 value")
	}
	return base64.RawURLEncoding.DecodeString(cleaned)
}

func GenerateVAPIDKeys() (publicKeyB64, privateKeyB64 string, err error) {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("failed generating P-256 keypair: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()),
		base64.RawURLEncoding.EncodeToString(priv.Bytes()),
		nil
}

func ValidateVAPIDPublicKey(pubB64 string) error {
	raw, err := DecodeURLSafeBase64(pubB64)
	if err != nil {
		return fmt.Errorf("invalid base64 VAPID public key: %w", err)
	}
	if len(raw) != 65 {
		return fmt.Errorf("VAPID public key must be 65 uncompressed bytes, got %d", len(raw))
	}
	if _, err := ecdh.P256().NewPublicKey(raw); err != nil {
		return fmt.Errorf("invalid P-256 VAPID public key: %w", err)
	}
	return nil
}

func ParseVAPIDPrivateKey(privB64 string) (*ecdsa.PrivateKey, string, error) {
	raw, err := DecodeURLSafeBase64(privB64)
	if err != nil {
		return nil, "", fmt.Errorf("invalid base64 VAPID private key: %w", err)
	}
	if len(raw) > 32 {
		return nil, "", fmt.Errorf("VAPID private key exceeds 32 bytes")
	}
	padded := make([]byte, 32)
	copy(padded[32-len(raw):], raw)

	ecdhPriv, err := ecdh.P256().NewPrivateKey(padded)
	if err != nil {
		return nil, "", fmt.Errorf("invalid P-256 VAPID private key: %w", err)
	}

	pubBytes := ecdhPriv.PublicKey().Bytes()
	if len(pubBytes) != 65 || pubBytes[0] != 0x04 {
		return nil, "", errors.New("unexpected uncompressed public key format")
	}

	ecdsaPriv := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(pubBytes[1:33]),
			Y:     new(big.Int).SetBytes(pubBytes[33:65]),
		},
		D: new(big.Int).SetBytes(padded),
	}

	return ecdsaPriv, base64.RawURLEncoding.EncodeToString(pubBytes), nil
}

func BuildVAPIDAuthHeader(endpoint, subject, pubKeyB64, privKeyB64 string, ttl time.Duration, now time.Time) (string, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", fmt.Errorf("invalid push subscription endpoint URL")
	}

	privKey, derivedPubB64, err := ParseVAPIDPrivateKey(privKeyB64)
	if err != nil {
		return "", err
	}

	pubKey := strings.TrimSpace(pubKeyB64)
	if pubKey == "" {
		pubKey = derivedPubB64
	} else {
		rawProvided, decErr := DecodeURLSafeBase64(pubKey)
		if decErr != nil {
			return "", fmt.Errorf("invalid VAPID public key: %w", decErr)
		}
		pubKey = base64.RawURLEncoding.EncodeToString(rawProvided)
		if pubKey != derivedPubB64 {
			return "", errors.New("VAPID public key does not match private key")
		}
	}

	sub := strings.TrimSpace(subject)
	if sub == "" {
		sub = "mailto:notifications@gocart.local"
	} else if !strings.HasPrefix(sub, "mailto:") && !strings.HasPrefix(sub, "https://") {
		return "", errors.New("VAPID subject must start with mailto: or https://")
	}

	if ttl <= 0 || ttl > 24*time.Hour {
		ttl = defaultVAPIDTTL
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	aud := u.Scheme + "://" + u.Host
	claims := jwt.MapClaims{
		"aud": aud,
		"exp": now.Add(ttl).Unix(),
		"sub": sub,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	signedJWT, err := token.SignedString(privKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign VAPID JWT: %w", err)
	}

	return fmt.Sprintf("vapid t=%s, k=%s", signedJWT, pubKey), nil
}

