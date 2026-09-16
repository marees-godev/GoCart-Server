package response

import (
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marees-godev/GoCart-Server/pkg/errors"
)

func TestErrorResponseFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, http.StatusBadRequest, "BAD_REQUEST", "Invalid body")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var res ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if res.Error.Code != "BAD_REQUEST" {
		t.Errorf("expected code 'BAD_REQUEST', got '%s'", res.Error.Code)
	}
	if res.Error.Message != "Invalid body" {
		t.Errorf("expected message 'Invalid body', got '%s'", res.Error.Message)
	}
}

func TestWriteErrorWithAppError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	appErr := errors.NotFound("user not found")
	WriteError(rec, req, appErr)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	var res ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if res.Error.Code != errors.CodeNotFound {
		t.Errorf("expected code %s, got %s", errors.CodeNotFound, res.Error.Code)
	}
	if res.Error.Message != "user not found" {
		t.Errorf("expected message 'user not found', got '%s'", res.Error.Message)
	}
}

func TestWriteErrorInternalSanitization(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	internalErr := errors.Internal(stdErrors.New("db connection failed"), "db secret failure details")
	WriteError(rec, req, internalErr)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}

	var res ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if res.Error.Code != errors.CodeInternalError {
		t.Errorf("expected code %s, got %s", errors.CodeInternalError, res.Error.Code)
	}
	if res.Error.Message != "An internal server error occurred" {
		t.Errorf("expected sanitized message, got '%s'", res.Error.Message)
	}
}
