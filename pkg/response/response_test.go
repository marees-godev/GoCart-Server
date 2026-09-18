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
	retErr := Error(rec, http.StatusBadRequest, "BAD_REQUEST", "Invalid body")

	if retErr == nil {
		t.Fatal("expected non-nil *ErrorResponse return value")
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var res ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if res.Error.Code != "BAD_REQUEST" || retErr.Error.Code != "BAD_REQUEST" {
		t.Errorf("expected code 'BAD_REQUEST', got res='%s' retErr='%s'", res.Error.Code, retErr.Error.Code)
	}
	if res.Error.Message != "Invalid body" || retErr.Error.Message != "Invalid body" {
		t.Errorf("expected message 'Invalid body', got res='%s' retErr='%s'", res.Error.Message, retErr.Error.Message)
	}
}

func TestWriteErrorWithAppError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	appErr := errors.NotFound("user not found")
	retErr := WriteError(rec, req, appErr)

	if retErr == nil {
		t.Fatal("expected non-nil *ErrorResponse return value")
	}

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	var res ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if res.Error.Code != errors.CodeNotFound || retErr.Error.Code != errors.CodeNotFound {
		t.Errorf("expected code %s, got res=%s retErr=%s", errors.CodeNotFound, res.Error.Code, retErr.Error.Code)
	}
	if res.Error.Message != "user not found" || retErr.Error.Message != "user not found" {
		t.Errorf("expected message 'user not found', got res='%s' retErr='%s'", res.Error.Message, retErr.Error.Message)
	}
}

func TestWriteErrorInternalSanitization(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	internalErr := errors.Internal(stdErrors.New("db connection failed"), "db secret failure details")
	retErr := WriteError(rec, req, internalErr)

	if retErr == nil {
		t.Fatal("expected non-nil *ErrorResponse return value")
	}

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}

	var res ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if res.Error.Code != errors.CodeInternalError || retErr.Error.Code != errors.CodeInternalError {
		t.Errorf("expected code %s, got res=%s retErr=%s", errors.CodeInternalError, res.Error.Code, retErr.Error.Code)
	}
	if res.Error.Message != "An internal server error occurred" || retErr.Error.Message != "An internal server error occurred" {
		t.Errorf("expected sanitized message, got res='%s' retErr='%s'", res.Error.Message, retErr.Error.Message)
	}

	nilRet := WriteError(rec, req, nil)
	if nilRet != nil {
		t.Errorf("expected nil *ErrorResponse for nil error input, got %v", nilRet)
	}
}
