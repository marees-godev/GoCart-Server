package grpcclient_test

import (
	"errors"
	"net/http"
	"testing"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTranslateGRPCError(t *testing.T) {
	tests := []struct {
		name         string
		input        error
		expectedCode string
		expectedHTTP int
		isNil        bool
	}{
		{
			name:  "Nil",
			input: nil,
			isNil: true,
		},
		{
			name:  "OK",
			input: status.Error(codes.OK, "ok"),
			isNil: true,
		},
		{
			name:         "NotFound",
			input:        status.Error(codes.NotFound, "item not found"),
			expectedCode: appErrors.CodeNotFound,
			expectedHTTP: http.StatusNotFound,
		},
		{
			name:         "InvalidArgument",
			input:        status.Error(codes.InvalidArgument, "invalid argument"),
			expectedCode: appErrors.CodeBadRequest,
			expectedHTTP: http.StatusBadRequest,
		},
		{
			name:         "Unauthenticated_Standard",
			input:        status.Error(codes.Unauthenticated, "not authenticated"),
			expectedCode: appErrors.CodeUnauthorized,
			expectedHTTP: http.StatusUnauthorized,
		},
		{
			name:         "Unauthenticated_InvalidCredentials",
			input:        status.Error(codes.Unauthenticated, "invalid credentials provided"),
			expectedCode: appErrors.CodeInvalidCredentials,
			expectedHTTP: http.StatusUnauthorized,
		},
		{
			name:         "Unauthenticated_Password",
			input:        status.Error(codes.Unauthenticated, "wrong password"),
			expectedCode: appErrors.CodeInvalidCredentials,
			expectedHTTP: http.StatusUnauthorized,
		},
		{
			name:         "PermissionDenied",
			input:        status.Error(codes.PermissionDenied, "forbidden"),
			expectedCode: appErrors.CodeForbidden,
			expectedHTTP: http.StatusForbidden,
		},
		{
			name:         "AlreadyExists",
			input:        status.Error(codes.AlreadyExists, "already exists"),
			expectedCode: appErrors.CodeConflict,
			expectedHTTP: http.StatusConflict,
		},
		{
			name:         "FailedPrecondition",
			input:        status.Error(codes.FailedPrecondition, "failed precondition"),
			expectedCode: appErrors.CodeUnprocessableEntity,
			expectedHTTP: http.StatusUnprocessableEntity,
		},
		{
			name:         "OutOfRange",
			input:        status.Error(codes.OutOfRange, "out of range"),
			expectedCode: appErrors.CodeUnprocessableEntity,
			expectedHTTP: http.StatusUnprocessableEntity,
		},
		{
			name:         "ResourceExhausted",
			input:        status.Error(codes.ResourceExhausted, "rate limited"),
			expectedCode: appErrors.CodeTooManyRequests,
			expectedHTTP: http.StatusTooManyRequests,
		},
		{
			name:         "DeadlineExceeded",
			input:        status.Error(codes.DeadlineExceeded, "context deadline exceeded"),
			expectedCode: "GATEWAY_TIMEOUT",
			expectedHTTP: http.StatusGatewayTimeout,
		},
		{
			name:         "Unavailable",
			input:        status.Error(codes.Unavailable, "service unavailable"),
			expectedCode: appErrors.CodeServiceUnavailable,
			expectedHTTP: http.StatusServiceUnavailable,
		},
		{
			name:         "Canceled",
			input:        status.Error(codes.Canceled, "request canceled"),
			expectedCode: "CLIENT_CLOSED_REQUEST",
			expectedHTTP: 499,
		},
		{
			name:         "AlreadyAppError",
			input:        appErrors.BadRequest("custom bad request"),
			expectedCode: appErrors.CodeBadRequest,
			expectedHTTP: http.StatusBadRequest,
		},
		{
			name:         "StandardError",
			input:        errors.New("generic downstream error"),
			expectedCode: appErrors.CodeInternalError,
			expectedHTTP: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := grpcclient.TranslateGRPCError(tt.input)
			if tt.isNil {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			appErr := appErrors.AsAppError(err)
			if appErr == nil {
				t.Fatalf("expected AppError, got %v", err)
			}
			if appErr.Code != tt.expectedCode {
				t.Errorf("expected code %s, got %s", tt.expectedCode, appErr.Code)
			}
			if appErr.HTTPStatus != tt.expectedHTTP {
				t.Errorf("expected HTTP status %d, got %d", tt.expectedHTTP, appErr.HTTPStatus)
			}
		})
	}
}

func TestToGRPCError(t *testing.T) {
	tests := []struct {
		name         string
		input        error
		expectedCode codes.Code
	}{
		{
			name:         "Nil",
			input:        nil,
			expectedCode: codes.OK,
		},
		{
			name:         "BadRequest",
			input:        appErrors.BadRequest("bad request"),
			expectedCode: codes.InvalidArgument,
		},
		{
			name:         "Unauthorized",
			input:        appErrors.Unauthorized("unauthorized"),
			expectedCode: codes.Unauthenticated,
		},
		{
			name:         "Forbidden",
			input:        appErrors.Forbidden("forbidden"),
			expectedCode: codes.PermissionDenied,
		},
		{
			name:         "NotFound",
			input:        appErrors.NotFound("not found"),
			expectedCode: codes.NotFound,
		},
		{
			name:         "Conflict",
			input:        appErrors.Conflict("conflict"),
			expectedCode: codes.AlreadyExists,
		},
		{
			name:         "UnprocessableEntity",
			input:        appErrors.UnprocessableEntity("unprocessable"),
			expectedCode: codes.FailedPrecondition,
		},
		{
			name:         "GenericError",
			input:        errors.New("standard error"),
			expectedCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcErr := grpcclient.ToGRPCError(tt.input)
			if tt.input == nil {
				if grpcErr != nil {
					t.Errorf("expected nil error, got %v", grpcErr)
				}
				return
			}
			st, ok := status.FromError(grpcErr)
			if !ok {
				t.Fatalf("expected gRPC status error, got %v", grpcErr)
			}
			if st.Code() != tt.expectedCode {
				t.Errorf("expected code %v, got %v", tt.expectedCode, st.Code())
			}
		})
	}
}
