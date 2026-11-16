package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFacebookVerifier_ExchangeCode(t *testing.T) {
	t.Run("missing app ID", func(t *testing.T) {
		v := NewHTTPFacebookTokenVerifier("", "secret")
		if _, err := v.ExchangeCode(context.Background(), "code", "redirect"); err == nil {
			t.Fatalf("expected error for missing app ID")
		}
	})

	t.Run("missing app secret", func(t *testing.T) {
		v := NewHTTPFacebookTokenVerifier("appid", "")
		if _, err := v.ExchangeCode(context.Background(), "code", "redirect"); err == nil {
			t.Fatalf("expected error for missing app secret")
		}
	})

	t.Run("empty code", func(t *testing.T) {
		v := NewHTTPFacebookTokenVerifier("appid", "secret")
		if _, err := v.ExchangeCode(context.Background(), "", "redirect"); err == nil {
			t.Fatalf("expected error for empty code")
		}
	})

	t.Run("success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"access_token": "fb-access-token-123",
			"token_type":   "bearer",
			"expires_in":   3600,
		})

		v := NewHTTPFacebookTokenVerifier("test-app-id", "test-secret")
		v.httpClient = &http.Client{
			Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			}),
		}

		token, err := v.ExchangeCode(context.Background(), "valid-code", "http://localhost/callback")
		if err != nil {
			t.Fatalf("unexpected error exchanging code: %v", err)
		}
		if token != "fb-access-token-123" {
			t.Fatalf("expected fb-access-token-123, got %s", token)
		}
	})

	t.Run("exchange error response", func(t *testing.T) {
		errBody, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": "code has expired",
			},
		})

		v := NewHTTPFacebookTokenVerifier("test-app-id", "test-secret")
		v.httpClient = &http.Client{
			Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewReader(errBody)),
					Header:     make(http.Header),
				}, nil
			}),
		}

		_, err := v.ExchangeCode(context.Background(), "expired-code", "http://localhost/callback")
		if err == nil || !strings.Contains(err.Error(), "code has expired") {
			t.Fatalf("expected error mentioning 'code has expired', got: %v", err)
		}
	})
}

func TestFacebookVerifier_Verify(t *testing.T) {
	t.Run("empty token", func(t *testing.T) {
		v := NewHTTPFacebookTokenVerifier("test-app-id", "test-secret")
		_, err := v.Verify(context.Background(), "")
		if err == nil {
			t.Fatalf("expected error for empty token")
		}
	})

	t.Run("missing configured app ID", func(t *testing.T) {
		v := NewHTTPFacebookTokenVerifier("", "test-secret")
		_, err := v.Verify(context.Background(), "some-token")
		if err == nil {
			t.Fatalf("expected error for missing app ID")
		}
	})

	t.Run("app ID mismatch", func(t *testing.T) {
		debugBody, _ := json.Marshal(map[string]any{
			"data": map[string]any{
				"app_id":     "different-app-id",
				"is_valid":   true,
				"user_id":    "fb-user-123",
				"expires_at": time.Now().Add(1 * time.Hour).Unix(),
			},
		})

		v := NewHTTPFacebookTokenVerifier("expected-app-id", "test-secret")
		v.httpClient = &http.Client{
			Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(debugBody)),
					Header:     make(http.Header),
				}, nil
			}),
		}

		_, err := v.Verify(context.Background(), "some-token")
		if err == nil || !strings.Contains(err.Error(), "app ID mismatch") {
			t.Fatalf("expected error for app ID mismatch, got %v", err)
		}
	})

	t.Run("token expired", func(t *testing.T) {
		debugBody, _ := json.Marshal(map[string]any{
			"data": map[string]any{
				"app_id":     "expected-app-id",
				"is_valid":   true,
				"user_id":    "fb-user-123",
				"expires_at": time.Now().Add(-1 * time.Hour).Unix(),
			},
		})

		v := NewHTTPFacebookTokenVerifier("expected-app-id", "test-secret")
		v.httpClient = &http.Client{
			Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(debugBody)),
					Header:     make(http.Header),
				}, nil
			}),
		}

		_, err := v.Verify(context.Background(), "some-token")
		if err == nil || !strings.Contains(err.Error(), "has expired") {
			t.Fatalf("expected error for expired token, got %v", err)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		debugBody, _ := json.Marshal(map[string]any{
			"data": map[string]any{
				"app_id":     "expected-app-id",
				"is_valid":   false,
				"user_id":    "fb-user-123",
				"expires_at": time.Now().Add(1 * time.Hour).Unix(),
			},
		})

		v := NewHTTPFacebookTokenVerifier("expected-app-id", "test-secret")
		v.httpClient = &http.Client{
			Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(debugBody)),
					Header:     make(http.Header),
				}, nil
			}),
		}

		_, err := v.Verify(context.Background(), "some-token")
		if err == nil || !strings.Contains(err.Error(), "invalid facebook token") {
			t.Fatalf("expected invalid facebook token, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		debugBody, _ := json.Marshal(map[string]any{
			"data": map[string]any{
				"app_id":     "expected-app-id",
				"is_valid":   true,
				"user_id":    "fb-user-123",
				"expires_at": time.Now().Add(1 * time.Hour).Unix(),
			},
		})
		meBody, _ := json.Marshal(map[string]any{
			"id":         "fb-user-123",
			"name":       "Alice Smith",
			"first_name": "Alice",
			"last_name":  "Smith",
			"email":      "alice@facebook.example.com",
		})

		v := NewHTTPFacebookTokenVerifier("expected-app-id", "test-secret")
		v.httpClient = &http.Client{
			Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "debug_token") {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader(debugBody)),
						Header:     make(http.Header),
					}, nil
				}
				if strings.Contains(req.URL.Path, "me") {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader(meBody)),
						Header:     make(http.Header),
					}, nil
				}
				return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewReader(nil))}, nil
			}),
		}

		payload, err := v.Verify(context.Background(), "valid-access-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if payload.UserID != "fb-user-123" {
			t.Fatalf("expected user ID fb-user-123, got %s", payload.UserID)
		}
		if payload.Email != "alice@facebook.example.com" {
			t.Fatalf("expected email alice@facebook.example.com, got %s", payload.Email)
		}
		if !payload.EmailVerified {
			t.Fatalf("expected emailVerified to be true")
		}
		if payload.FirstName != "Alice" || payload.LastName != "Smith" {
			t.Fatalf("expected Alice Smith, got %s %s", payload.FirstName, payload.LastName)
		}
	})
}
