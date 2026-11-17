package gstin

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/services/store-service/internal/config"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestVerifyGSTIN_Valid(t *testing.T) {
	mockResponse := `{
		"success": true,
		"gstin": "33AAACC1206D1ZN",
		"data": {
			"gstin": "33AAACC1206D1ZN",
			"legal_name": "CENTRAL WAREHOUSING CORPORATION",
			"status": "Active",
			"block_status": "Unblocked"
		}
	}`

	mockTransport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("x-api-key") != "test-api-key" {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
				Header:     make(http.Header),
			}, nil
		}
		if r.URL.Path != "/gstin/33AAACC1206D1ZN" {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("not found")),
				Header:     make(http.Header),
			}, nil
		}
		h := make(http.Header)
		h.Set("Content-Type", "application/json")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(mockResponse)),
			Header:     h,
		}, nil
	})

	httpClient := &http.Client{Transport: mockTransport}

	client := NewGSTINClient(config.GSTINConfig{
		APIKey:  "test-api-key",
		BaseURL: "https://mock.gstinapi.in",
		Enabled: true,
		Timeout: 2 * time.Second,
	}, httpClient)

	res, err := client.VerifyGSTIN(context.Background(), "33AAACC1206D1ZN")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !res.Success {
		t.Errorf("expected success true, got false")
	}
	if res.Data == nil || res.Data.LegalName != "CENTRAL WAREHOUSING CORPORATION" {
		t.Errorf("unexpected data returned: %+v", res.Data)
	}
}

func TestVerifyGSTIN_InvalidFormat(t *testing.T) {
	client := NewGSTINClient(config.GSTINConfig{
		Enabled: true,
	})

	_, err := client.VerifyGSTIN(context.Background(), "INVALID123")
	if err == nil {
		t.Errorf("expected error for invalid format, got nil")
	}
}

func TestVerifyGSTIN_APIError(t *testing.T) {
	mockTransport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("server error")),
			Header:     make(http.Header),
		}, nil
	})

	httpClient := &http.Client{Transport: mockTransport}

	client := NewGSTINClient(config.GSTINConfig{
		BaseURL: "https://mock.gstinapi.in",
		Timeout: 2 * time.Second,
	}, httpClient)

	_, err := client.VerifyGSTIN(context.Background(), "33AAACC1206D1ZN")
	if err == nil {
		t.Errorf("expected error for HTTP 500, got nil")
	}
}
