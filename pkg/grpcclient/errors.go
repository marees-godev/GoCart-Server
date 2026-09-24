package grpcclient

import (
	"errors"
	"net/http"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TranslateGRPCError translates a gRPC status error into an application-specific AppError.
func TranslateGRPCError(err error) error {
	if err == nil {
		return nil
	}

	var appErr *appErrors.AppError
	if errors.As(err, &appErr) {
		return appErr
	}

	st, ok := status.FromError(err)
	if !ok {
		return appErrors.Internal(err, "unexpected downstream error")
	}

	msg := st.Message()
	if msg == "" {
		msg = st.Code().String()
	}

	switch st.Code() {
	case codes.OK:
		return nil
	case codes.NotFound:
		return appErrors.NotFound(msg)
	case codes.InvalidArgument:
		return appErrors.BadRequest(msg)
	case codes.AlreadyExists:
		return appErrors.Conflict(msg)
	case codes.Unauthenticated:
		return appErrors.Unauthorized(msg)
	case codes.PermissionDenied:
		return appErrors.Forbidden(msg)
	case codes.FailedPrecondition, codes.OutOfRange:
		return appErrors.UnprocessableEntity(msg)
	case codes.DeadlineExceeded:
		return appErrors.New("GATEWAY_TIMEOUT", "downstream service request timed out", http.StatusGatewayTimeout)
	case codes.Unavailable:
		return appErrors.ServiceUnavailable("downstream service is unavailable")
	case codes.Canceled:
		return appErrors.New("CLIENT_CLOSED_REQUEST", "client request was canceled", 499)
	default:
		return appErrors.Internal(err, "downstream service error")
	}
}

// ToGRPCError translates an AppError or standard error into a gRPC status error.
func ToGRPCError(err error) error {
	if err == nil {
		return nil
	}
	appErr := appErrors.AsAppError(err)
	if appErr == nil {
		return status.Error(codes.Internal, err.Error())
	}
	switch appErr.Code {
	case appErrors.CodeBadRequest:
		return status.Error(codes.InvalidArgument, appErr.Message)
	case appErrors.CodeUnauthorized:
		return status.Error(codes.Unauthenticated, appErr.Message)
	case appErrors.CodeForbidden:
		return status.Error(codes.PermissionDenied, appErr.Message)
	case appErrors.CodeNotFound:
		return status.Error(codes.NotFound, appErr.Message)
	case appErrors.CodeConflict:
		return status.Error(codes.AlreadyExists, appErr.Message)
	case appErrors.CodeUnprocessableEntity:
		return status.Error(codes.FailedPrecondition, appErr.Message)
	default:
		return status.Error(codes.Internal, appErr.ClientMessage())
	}
}
