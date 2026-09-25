package gstin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/services/store-service/internal/config"
)

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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-api-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/gstin/33AAACC1206D1ZN" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	client := NewGSTINClient(config.GSTINConfig{
		APIKey:  "test-api-key",
		BaseURL: server.URL,
		Enabled: true,
		Timeout: 2 * time.Second,
	})

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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewGSTINClient(config.GSTINConfig{
		BaseURL: server.URL,
		Timeout: 2 * time.Second,
	})

	_, err := client.VerifyGSTIN(context.Background(), "33AAACC1206D1ZN")
	if err == nil {
		t.Errorf("expected error for HTTP 500, got nil")
	}
}
