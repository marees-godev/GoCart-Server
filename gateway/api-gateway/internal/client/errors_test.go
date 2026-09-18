package client

import (
	"errors"
	"net/http"
	"testing"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTranslateGRPCError(t *testing.T) {
	tests := []struct {
		name         string
		inputErr     error
		expectedCode string
		expectedHTTP int
		isNil        bool
	}{
		{
			name:     "nil error",
			inputErr: nil,
			isNil:    true,
		},
		{
			name:     "grpc OK code",
			inputErr: status.Error(codes.OK, "ok"),
			isNil:    true,
		},
		{
			name:         "grpc NotFound",
			inputErr:     status.Error(codes.NotFound, "cart item not found"),
			expectedCode: appErrors.CodeNotFound,
			expectedHTTP: http.StatusNotFound,
		},
		{
			name:         "grpc InvalidArgument",
			inputErr:     status.Error(codes.InvalidArgument, "invalid quantity"),
			expectedCode: appErrors.CodeBadRequest,
			expectedHTTP: http.StatusBadRequest,
		},
		{
			name:         "grpc AlreadyExists",
			inputErr:     status.Error(codes.AlreadyExists, "item already in cart"),
			expectedCode: appErrors.CodeConflict,
			expectedHTTP: http.StatusConflict,
		},
		{
			name:         "grpc Unauthenticated",
			inputErr:     status.Error(codes.Unauthenticated, "invalid token"),
			expectedCode: appErrors.CodeUnauthorized,
			expectedHTTP: http.StatusUnauthorized,
		},
		{
			name:         "grpc PermissionDenied",
			inputErr:     status.Error(codes.PermissionDenied, "access denied"),
			expectedCode: appErrors.CodeForbidden,
			expectedHTTP: http.StatusForbidden,
		},
		{
			name:         "grpc FailedPrecondition",
			inputErr:     status.Error(codes.FailedPrecondition, "stock unavailable"),
			expectedCode: appErrors.CodeUnprocessableEntity,
			expectedHTTP: http.StatusUnprocessableEntity,
		},
		{
			name:         "grpc DeadlineExceeded",
			inputErr:     status.Error(codes.DeadlineExceeded, "context deadline exceeded"),
			expectedCode: "GATEWAY_TIMEOUT",
			expectedHTTP: http.StatusGatewayTimeout,
		},
		{
			name:         "grpc Unavailable",
			inputErr:     status.Error(codes.Unavailable, "service unavailable"),
			expectedCode: appErrors.CodeServiceUnavailable,
			expectedHTTP: http.StatusServiceUnavailable,
		},
		{
			name:         "already an AppError",
			inputErr:     appErrors.BadRequest("custom bad request"),
			expectedCode: appErrors.CodeBadRequest,
			expectedHTTP: http.StatusBadRequest,
		},
		{
			name:         "non-grpc standard error",
			inputErr:     errors.New("generic error"),
			expectedCode: appErrors.CodeInternalError,
			expectedHTTP: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := TranslateGRPCError(tt.inputErr)
			if tt.isNil {
				if res != nil {
					t.Fatalf("expected nil error, got: %v", res)
				}
				return
			}

			var appErr *appErrors.AppError
			if !errors.As(res, &appErr) {
				t.Fatalf("expected result to be *appErrors.AppError, got %T: %v", res, res)
			}

			if appErr.Code != tt.expectedCode {
				t.Errorf("expected code %q, got %q", tt.expectedCode, appErr.Code)
			}
			if appErr.HTTPStatus != tt.expectedHTTP {
				t.Errorf("expected HTTP status %d, got %d", tt.expectedHTTP, appErr.HTTPStatus)
			}
		})
	}
}
