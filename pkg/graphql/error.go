package graphql

import (
	"context"
	"errors"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"go.opentelemetry.io/otel/trace"
)

type Location struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type GraphQLError struct {
	Message    string         `json:"message"`
	Path       []any          `json:"path,omitempty"`
	Locations  []Location     `json:"locations,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

type GraphQLErrorResponse struct {
	Errors []GraphQLError `json:"errors"`
}

func FormatErrorResponse(err error) *appErrors.ErrorResponse {
	if err == nil {
		return appErrors.NewErrorResponse(appErrors.CodeInternalError, "An internal server error occurred")
	}
	appErr := appErrors.AsAppError(err)
	return appErr.ToResponse()
}

func FormatError(ctx context.Context, err error) GraphQLError {
	if err == nil {
		return GraphQLError{
			Message: "An unknown error occurred",
			Extensions: map[string]any{
				"code":       appErrors.CodeInternalError,
				"request_id": GetRequestID(ctx),
				"trace_id":   GetTraceID(ctx),
			},
		}
	}

	appErr := appErrors.AsAppError(err)

	code := appErr.Code
	message := appErr.ClientMessage()

	extensions := map[string]any{
		"code": code,
	}

	if reqID := GetRequestID(ctx); reqID != "" {
		extensions["request_id"] = reqID
	}
	if traceID := GetTraceID(ctx); traceID != "" {
		extensions["trace_id"] = traceID
	}

	return GraphQLError{
		Message:    message,
		Extensions: extensions,
	}
}

func GetRequestID(ctx context.Context) string {
	return middleware.GetRequestID(ctx)
}

func GetTraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

func IsAuthError(err error) bool {
	var appErr *appErrors.AppError
	if errors.As(err, &appErr) {
		return appErr.Code == appErrors.CodeUnauthorized || appErr.Code == appErrors.CodeForbidden
	}
	return false
}
