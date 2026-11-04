package gstin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/config"
)

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestVerifyGSTIN_APIError(t *testing.T) {
	mockHTTP := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":"internal error"}`)),
			}
		}),
	}

	c := NewGSTINClient(config.GSTINConfig{
		BaseURL: "http://example.com/api",
		APIKey:  "test-api-key",
		Enabled: true,
		Timeout: 2 * time.Second,
	}, mockHTTP)

	_, err := c.VerifyGSTIN(context.Background(), "33AAACC1206D1ZN")
	if err == nil {
		t.Fatalf("expected error on HTTP 500, got nil")
	}
}

func TestVerifyGSTIN_Unauthorized(t *testing.T) {
	mockHTTP := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":"invalid API key"}`)),
			}
		}),
	}

	c := NewGSTINClient(config.GSTINConfig{
		BaseURL: "http://example.com/api",
		APIKey:  "bad-key",
		Enabled: true,
		Timeout: 2 * time.Second,
	}, mockHTTP)

	_, err := c.VerifyGSTIN(context.Background(), "33AAACC1206D1ZN")
	if err == nil || !strings.Contains(err.Error(), "invalid credential") {
		t.Fatalf("expected invalid credential error on HTTP 401, got %v", err)
	}
}

func TestVerifyGSTIN_InvalidFormat(t *testing.T) {
	c := NewGSTINClient(config.GSTINConfig{
		Enabled: true,
	})

	_, err := c.VerifyGSTIN(context.Background(), "INVALID123")
	if err == nil || !strings.Contains(err.Error(), "invalid GSTIN format") {
		t.Errorf("expected invalid GSTIN format error, got %v", err)
	}
}

func TestVerifyGSTIN_Disabled(t *testing.T) {
	c := NewGSTINClient(config.GSTINConfig{
		Enabled: false,
	})

	if c.IsEnabled() {
		t.Errorf("expected client to be disabled")
	}

	_, err := c.VerifyGSTIN(context.Background(), "33AAACC1206D1ZN")
	if !errors.Is(err, ErrGSTINVerificationDisabled) {
		t.Fatalf("expected ErrGSTINVerificationDisabled, got %v", err)
	}
}

func TestVerifyGSTIN_API(t *testing.T) {
	mockResponse := `{
		"success": true,
		"gstin": "33AAACC1206D1ZN",
		"data": {
			"gstin": "33AAACC1206D1ZN",
			"legal_name": "CENTRAL LOGISTICS CORP",
			"status": "Active",
			"block_status": "Clean"
		}
	}`

	mockHTTP := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(mockResponse)),
			}
		}),
	}

	c := NewGSTINClient(config.GSTINConfig{
		BaseURL: "http://example.com/api",
		APIKey:  "test-api-key",
		Enabled: true,
		Timeout: 2 * time.Second,
	}, mockHTTP)

	res, err := c.VerifyGSTIN(context.Background(), "33AAACC1206D1ZN")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !res.Success || res.Data == nil || res.Data.LegalName != "CENTRAL LOGISTICS CORP" {
		t.Errorf("unexpected response: %+v", res)
	}
}
