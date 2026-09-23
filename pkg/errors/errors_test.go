package errors

import (
	"errors"
	"net/http"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAppErrorConstructors(t *testing.T) {
	tests := []struct {
		name           string
		err            *AppError
		expectedCode   string
		expectedStatus int
		expectedMsg    string
	}{
		{
			name:           "BadRequest",
			err:            BadRequest("invalid input"),
			expectedCode:   CodeBadRequest,
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "invalid input",
		},
		{
			name:           "Unauthorized",
			err:            Unauthorized("token missing"),
			expectedCode:   CodeUnauthorized,
			expectedStatus: http.StatusUnauthorized,
			expectedMsg:    "token missing",
		},
		{
			name:           "Forbidden",
			err:            Forbidden("permission denied"),
			expectedCode:   CodeForbidden,
			expectedStatus: http.StatusForbidden,
			expectedMsg:    "permission denied",
		},
		{
			name:           "NotFound",
			err:            NotFound("user not found"),
			expectedCode:   CodeNotFound,
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "user not found",
		},
		{
			name:           "Internal",
			err:            Internal(errors.New("db error"), "internal error"),
			expectedCode:   CodeInternalError,
			expectedStatus: http.StatusInternalServerError,
			expectedMsg:    "An internal server error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.expectedCode {
				t.Errorf("expected code %s, got %s", tt.expectedCode, tt.err.Code)
			}
			if tt.err.HTTPStatus != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, tt.err.HTTPStatus)
			}
			if msg := tt.err.ClientMessage(); msg != tt.expectedMsg {
				t.Errorf("expected client message %s, got %s", tt.expectedMsg, msg)
			}
		})
	}
}

func TestAsAppError(t *testing.T) {
	stdErr := errors.New("raw standard error")
	appErr := AsAppError(stdErr)

	if appErr.Code != CodeInternalError {
		t.Errorf("expected CodeInternalError, got %s", appErr.Code)
	}
	if appErr.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("expected 500 status, got %d", appErr.HTTPStatus)
	}

	customErr := BadRequest("bad query")
	converted := AsAppError(customErr)
	if converted.Code != CodeBadRequest {
		t.Errorf("expected CodeBadRequest, got %s", converted.Code)
	}
}

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse(CodeBadRequest, "invalid payload")
	if resp == nil {
		t.Fatal("expected non-nil *ErrorResponse")
	}
	if resp.Error.Code != CodeBadRequest {
		t.Errorf("expected code %s, got %s", CodeBadRequest, resp.Error.Code)
	}
	if resp.Error.Message != "invalid payload" {
		t.Errorf("expected message 'invalid payload', got %s", resp.Error.Message)
	}
}

func TestToResponse(t *testing.T) {
	appErr := NotFound("product not found")
	resp := appErr.ToResponse()
	if resp == nil {
		t.Fatal("expected non-nil *ErrorResponse")
	}
	if resp.Error.Code != CodeNotFound {
		t.Errorf("expected code %s, got %s", CodeNotFound, resp.Error.Code)
	}
	if resp.Error.Message != "product not found" {
		t.Errorf("expected message 'product not found', got %s", resp.Error.Message)
	}
}

func TestMapAppErrorToGRPC(t *testing.T) {
	if err := MapAppErrorToGRPC(nil); err != nil {
		t.Errorf("expected nil for nil error, got %v", err)
	}

	tests := []struct {
		name         string
		err          error
		expectedCode codes.Code
		expectedMsg  string
	}{
		{
			name:         "NotFound",
			err:          NotFound("item not found"),
			expectedCode: codes.NotFound,
			expectedMsg:  "item not found",
		},
		{
			name:         "Unauthorized",
			err:          Unauthorized("unauthorized token"),
			expectedCode: codes.Unauthenticated,
			expectedMsg:  "unauthorized token",
		},
		{
			name:         "Forbidden",
			err:          Forbidden("access denied"),
			expectedCode: codes.PermissionDenied,
			expectedMsg:  "access denied",
		},
		{
			name:         "BadRequest",
			err:          BadRequest("invalid field"),
			expectedCode: codes.InvalidArgument,
			expectedMsg:  "invalid field",
		},
		{
			name:         "UnprocessableEntity",
			err:          UnprocessableEntity("unprocessable content"),
			expectedCode: codes.InvalidArgument,
			expectedMsg:  "unprocessable content",
		},
		{
			name:         "Conflict",
			err:          Conflict("already exists"),
			expectedCode: codes.AlreadyExists,
			expectedMsg:  "already exists",
		},
		{
			name:         "TooManyRequests",
			err:          TooManyRequests("rate limit exceeded"),
			expectedCode: codes.ResourceExhausted,
			expectedMsg:  "rate limit exceeded",
		},
		{
			name:         "ServiceUnavailable",
			err:          ServiceUnavailable("down for maintenance"),
			expectedCode: codes.Unavailable,
			expectedMsg:  "down for maintenance",
		},
		{
			name:         "Internal",
			err:          Internal(errors.New("db down"), "db failed"),
			expectedCode: codes.Internal,
			expectedMsg:  "db failed",
		},
		{
			name:         "StandardError",
			err:          errors.New("generic error"),
			expectedCode: codes.Internal,
			expectedMsg:  "generic error",
		},
		{
			name:         "AlreadyGRPCStatus",
			err:          status.Error(codes.DeadlineExceeded, "timeout"),
			expectedCode: codes.DeadlineExceeded,
			expectedMsg:  "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcErr := MapAppErrorToGRPC(tt.err)
			st, ok := status.FromError(grpcErr)
			if !ok {
				t.Fatalf("expected valid gRPC status from MapAppErrorToGRPC, got %v", grpcErr)
			}
			if st.Code() != tt.expectedCode {
				t.Errorf("expected code %v, got %v", tt.expectedCode, st.Code())
			}
			if st.Message() != tt.expectedMsg {
				t.Errorf("expected msg %q, got %q", tt.expectedMsg, st.Message())
			}
		})
	}
}

func TestAppError_ToGRPC(t *testing.T) {
	appErr := NotFound("user not found")
	grpcErr := appErr.ToGRPC()
	st, ok := status.FromError(grpcErr)
	if !ok || st.Code() != codes.NotFound {
		t.Fatalf("expected codes.NotFound, got %v", grpcErr)
	}

	aliasErr := ToGRPC(appErr)
	stAlias, ok := status.FromError(aliasErr)
	if !ok || stAlias.Code() != codes.NotFound {
		t.Fatalf("expected codes.NotFound from ToGRPC alias, got %v", aliasErr)
	}
}
