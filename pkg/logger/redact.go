package logger

import (
	"log/slog"
	"strings"
)

var sensitiveKeys = map[string]struct{}{
	"password":           {},
	"password_hash":      {},
	"old_password":       {},
	"new_password":       {},
	"token":              {},
	"access_token":       {},
	"refresh_token":      {},
	"token_hash":         {},
	"secret":             {},
	"client_secret":      {},
	"api_key":            {},
	"authorization":      {},
	"auth_header":        {},
	"credit_card":        {},
	"card_number":        {},
	"cvv":                {},
	"cvc":                {},
	"pin":                {},
	"payment_credential": {},
	"gateway_secret":     {},
	"signature":          {},
	"private_key":        {},
}

const redactedValue = "[REDACTED]"

func RedactAttr(groups []string, a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	if isSensitiveKey(key) {
		return slog.String(a.Key, redactedValue)
	}

	switch a.Value.Kind() {
	case slog.KindGroup:
		attrs := a.Value.Group()
		redactedGroup := make([]slog.Attr, len(attrs))
		for i, attr := range attrs {
			redactedGroup[i] = RedactAttr(append(groups, a.Key), attr)
		}
		return slog.Attr{
			Key:   a.Key,
			Value: slog.GroupValue(redactedGroup...),
		}
	case slog.KindString:
		val := a.Value.String()
		if isSensitiveValue(val) {
			return slog.String(a.Key, redactedValue)
		}
	}

	return a
}

func isSensitiveKey(key string) bool {
	cleanKey := strings.ReplaceAll(strings.ToLower(key), "-", "_")
	if _, exists := sensitiveKeys[cleanKey]; exists {
		return true
	}
	for sensitive := range sensitiveKeys {
		if strings.Contains(cleanKey, sensitive) {
			return true
		}
	}
	return false
}

func isSensitiveValue(val string) bool {
	lowerVal := strings.ToLower(val)
	if strings.HasPrefix(lowerVal, "bearer ") {
		return true
	}
	return false
}
