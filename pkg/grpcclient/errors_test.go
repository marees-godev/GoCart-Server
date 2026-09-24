package grpcclient_test

import (
	"errors"
	"testing"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTranslateGRPCError(t *testing.T) {
	tests := []struct {
		name     string
		input    error
		expected string
	}{
		{
			name:     "NotFound",
			input:    status.Error(codes.NotFound, "item not found"),
			expected: appErrors.CodeNotFound,
		},
		{
			name:     "InvalidArgument",
			input:    status.Error(codes.InvalidArgument, "invalid argument"),
			expected: appErrors.CodeBadRequest,
		},
		{
			name:     "Unauthenticated",
			input:    status.Error(codes.Unauthenticated, "not authenticated"),
			expected: appErrors.CodeUnauthorized,
		},
		{
			name:     "PermissionDenied",
			input:    status.Error(codes.PermissionDenied, "forbidden"),
			expected: appErrors.CodeForbidden,
		},
		{
			name:     "AlreadyExists",
			input:    status.Error(codes.AlreadyExists, "already exists"),
			expected: appErrors.CodeConflict,
		},
		{
			name:     "FailedPrecondition",
			input:    status.Error(codes.FailedPrecondition, "failed precondition"),
			expected: appErrors.CodeUnprocessableEntity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := grpcclient.TranslateGRPCError(tt.input)
			appErr := appErrors.AsAppError(err)
			if appErr == nil {
				t.Fatalf("expected AppError, got %v", err)
			}
			if appErr.Code != tt.expected {
				t.Errorf("expected code %s, got %s", tt.expected, appErr.Code)
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
