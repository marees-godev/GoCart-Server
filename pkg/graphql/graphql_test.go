package graphql

import (
	"context"
	"errors"
	"testing"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
)

func TestFormatErrorUnauthorized(t *testing.T) {
	ctx := context.Background()
	ctx = logger.WithRequestID(ctx, "req-123")

	authErr := appErrors.Unauthorized("token invalid or expired")
	gqlErr := FormatError(ctx, authErr)

	if gqlErr.Message != "token invalid or expired" {
		t.Errorf("expected message 'token invalid or expired', got '%s'", gqlErr.Message)
	}

	if gqlErr.Extensions["code"] != appErrors.CodeUnauthorized {
		t.Errorf("expected code UNAUTHORIZED, got '%v'", gqlErr.Extensions["code"])
	}

	if gqlErr.Extensions["request_id"] != "req-123" {
		t.Errorf("expected request_id 'req-123', got '%v'", gqlErr.Extensions["request_id"])
	}

	if !IsAuthError(authErr) {
		t.Error("expected IsAuthError to return true for unauthorized error")
	}
}

func TestFormatErrorForbidden(t *testing.T) {
	forbiddenErr := appErrors.Forbidden("insufficient permissions")
	if !IsAuthError(forbiddenErr) {
		t.Error("expected IsAuthError to return true for forbidden error")
	}

	gqlErr := FormatError(context.Background(), forbiddenErr)
	if gqlErr.Extensions["code"] != appErrors.CodeForbidden {
		t.Errorf("expected code FORBIDDEN, got '%v'", gqlErr.Extensions["code"])
	}
}

func TestFormatErrorInternalSanitization(t *testing.T) {
	ctx := context.Background()
	ctx = logger.WithRequestID(ctx, "req-999")

	internalErr := appErrors.Internal(errors.New("sql statement syntax error"), "secret internal info")
	gqlErr := FormatError(ctx, internalErr)

	if gqlErr.Message != "An internal server error occurred" {
		t.Errorf("expected sanitized message 'An internal server error occurred', got '%s'", gqlErr.Message)
	}

	if gqlErr.Extensions["code"] != appErrors.CodeInternalError {
		t.Errorf("expected code INTERNAL_SERVER_ERROR, got '%v'", gqlErr.Extensions["code"])
	}

	if gqlErr.Extensions["request_id"] != "req-999" {
		t.Errorf("expected request_id 'req-999', got '%v'", gqlErr.Extensions["request_id"])
	}
}

func TestFormatErrorResponse(t *testing.T) {
	err := appErrors.NotFound("item not found")
	res := FormatErrorResponse(err)
	if res == nil {
		t.Fatal("expected non-nil *appErrors.ErrorResponse")
	}
	if res.Error.Code != appErrors.CodeNotFound {
		t.Errorf("expected code %s, got %s", appErrors.CodeNotFound, res.Error.Code)
	}
	if res.Error.Message != "item not found" {
		t.Errorf("expected message 'item not found', got %s", res.Error.Message)
	}

	nilRes := FormatErrorResponse(nil)
	if nilRes == nil {
		t.Fatal("expected non-nil *appErrors.ErrorResponse for nil error")
	}
	if nilRes.Error.Code != appErrors.CodeInternalError {
		t.Errorf("expected code %s, got %s", appErrors.CodeInternalError, nilRes.Error.Code)
	}
}
