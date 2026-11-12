package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func TestHTTPGoogleTokenVerifier_Verify(t *testing.T) {
	t.Run("valid token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"sub":            "google-sub-123",
			"email":          "test@example.com",
			"email_verified": "true",
			"given_name":     "John",
			"family_name":    "Doe",
			"aud":            "client-id-xyz",
			"iss":            "https://accounts.google.com",
			"exp":            time.Now().Add(1 * time.Hour).Unix(),
		})

		verifier := &HTTPGoogleTokenVerifier{
			clientID: "client-id-xyz",
			httpClient: &http.Client{
				Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
					token := req.URL.Query().Get("id_token")
					if token != "valid-token" {
						errBody, _ := json.Marshal(map[string]string{"error_description": "bad token"})
						return &http.Response{
							StatusCode: http.StatusBadRequest,
							Body:       io.NopCloser(bytes.NewReader(errBody)),
							Header:     make(http.Header),
						}, nil
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader(body)),
						Header:     make(http.Header),
					}, nil
				}),
			},
		}

		payload, err := verifier.Verify(context.Background(), "valid-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if payload.Subject != "google-sub-123" {
			t.Errorf("expected sub google-sub-123, got %s", payload.Subject)
		}
		if payload.Email != "test@example.com" {
			t.Errorf("expected email test@example.com, got %s", payload.Email)
		}
		if !payload.EmailVerified {
			t.Errorf("expected email_verified true, got false")
		}
		if payload.FirstName != "John" || payload.LastName != "Doe" {
			t.Errorf("unexpected names: %s %s", payload.FirstName, payload.LastName)
		}
	})

	t.Run("empty token", func(t *testing.T) {
		verifier := NewHTTPGoogleTokenVerifier("client-id-xyz")
		_, err := verifier.Verify(context.Background(), "")
		if err == nil {
			t.Fatalf("expected error for empty token")
		}
	})

	t.Run("missing configured client ID", func(t *testing.T) {
		verifier := NewHTTPGoogleTokenVerifier("")
		_, err := verifier.Verify(context.Background(), "some-token")
		if err == nil || err.Error() != "google client ID is not configured" {
			t.Fatalf("expected 'google client ID is not configured', got: %v", err)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"sub":            "google-sub-123",
			"email":          "test@example.com",
			"email_verified": true,
			"aud":            "client-id-xyz",
			"iss":            "https://accounts.google.com",
			"exp":            time.Now().Add(-1 * time.Hour).Unix(),
		})

		verifier := &HTTPGoogleTokenVerifier{
			clientID: "client-id-xyz",
			httpClient: &http.Client{
				Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader(body)),
						Header:     make(http.Header),
					}, nil
				}),
			},
		}

		_, err := verifier.Verify(context.Background(), "expired-token")
		if err == nil || err.Error() != "google token has expired" {
			t.Fatalf("expected 'google token has expired', got: %v", err)
		}
	})

	t.Run("unverified email", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"sub":            "google-sub-123",
			"email":          "test@example.com",
			"email_verified": false,
			"aud":            "client-id-xyz",
			"iss":            "https://accounts.google.com",
			"exp":            time.Now().Add(1 * time.Hour).Unix(),
		})

		verifier := &HTTPGoogleTokenVerifier{
			clientID: "client-id-xyz",
			httpClient: &http.Client{
				Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader(body)),
						Header:     make(http.Header),
					}, nil
				}),
			},
		}

		_, err := verifier.Verify(context.Background(), "unverified-token")
		if err == nil || err.Error() != "google email is not verified" {
			t.Fatalf("expected 'google email is not verified', got: %v", err)
		}
	})

	t.Run("client ID mismatch and empty audience", func(t *testing.T) {
		for _, aud := range []string{"other-client-id", ""} {
			body, _ := json.Marshal(map[string]any{
				"sub":            "google-sub-123",
				"email":          "test@example.com",
				"email_verified": true,
				"aud":            aud,
				"iss":            "https://accounts.google.com",
				"exp":            time.Now().Add(1 * time.Hour).Unix(),
			})

			verifier := &HTTPGoogleTokenVerifier{
				clientID: "expected-client-id",
				httpClient: &http.Client{
					Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewReader(body)),
							Header:     make(http.Header),
						}, nil
					}),
				},
			}

			_, err := verifier.Verify(context.Background(), "mismatched-token")
			if err == nil {
				t.Fatalf("expected client ID mismatch error for aud=%q", aud)
			}
		}
	})

	t.Run("invalid issuer", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"sub":            "google-sub-123",
			"email":          "test@example.com",
			"email_verified": true,
			"aud":            "client-id-xyz",
			"iss":            "https://evil.example.com",
			"exp":            time.Now().Add(1 * time.Hour).Unix(),
		})

		verifier := &HTTPGoogleTokenVerifier{
			clientID: "client-id-xyz",
			httpClient: &http.Client{
				Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader(body)),
						Header:     make(http.Header),
					}, nil
				}),
			},
		}

		_, err := verifier.Verify(context.Background(), "bad-issuer-token")
		if err == nil {
			t.Fatalf("expected invalid issuer error")
		}
	})
}
