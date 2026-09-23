package errors

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	CodeBadRequest          = "BAD_REQUEST"
	CodeUnauthorized        = "UNAUTHORIZED"
	CodeForbidden           = "FORBIDDEN"
	CodeNotFound            = "NOT_FOUND"
	CodeConflict            = "CONFLICT"
	CodeUnprocessableEntity = "UNPROCESSABLE_ENTITY"
	CodeInternalError       = "INTERNAL_SERVER_ERROR"
	CodeServiceUnavailable  = "SERVICE_UNAVAILABLE"
	CodeTooManyRequests     = "TOO_MANY_REQUESTS"
)

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

func NewErrorResponse(code, message string) *ErrorResponse {
	return &ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	}
}

type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
	Err        error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func (e *AppError) ClientMessage() string {
	if e.HTTPStatus >= http.StatusInternalServerError || e.Code == CodeInternalError {
		return "An internal server error occurred"
	}
	return e.Message
}

func (e *AppError) ToResponse() *ErrorResponse {
	return &ErrorResponse{
		Error: ErrorDetail{
			Code:    e.Code,
			Message: e.ClientMessage(),
		},
	}
}

func New(code, message string, status int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: status,
	}
}

func Wrap(err error, code, message string, status int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: status,
		Err:        err,
	}
}

func BadRequest(message string) *AppError {
	if message == "" {
		message = "Bad request"
	}
	return New(CodeBadRequest, message, http.StatusBadRequest)
}

func Unauthorized(message string) *AppError {
	if message == "" {
		message = "Unauthorized access"
	}
	return New(CodeUnauthorized, message, http.StatusUnauthorized)
}

func Forbidden(message string) *AppError {
	if message == "" {
		message = "Access forbidden"
	}
	return New(CodeForbidden, message, http.StatusForbidden)
}

func NotFound(message string) *AppError {
	if message == "" {
		message = "Resource not found"
	}
	return New(CodeNotFound, message, http.StatusNotFound)
}

func Conflict(message string) *AppError {
	if message == "" {
		message = "Resource conflict"
	}
	return New(CodeConflict, message, http.StatusConflict)
}

func UnprocessableEntity(message string) *AppError {
	if message == "" {
		message = "Unprocessable entity"
	}
	return New(CodeUnprocessableEntity, message, http.StatusUnprocessableEntity)
}

func Internal(err error, message string) *AppError {
	if message == "" {
		message = "An internal server error occurred"
	}
	return Wrap(err, CodeInternalError, message, http.StatusInternalServerError)
}

func ServiceUnavailable(message string) *AppError {
	if message == "" {
		message = "Service temporarily unavailable"
	}
	return New(CodeServiceUnavailable, message, http.StatusServiceUnavailable)
}

func TooManyRequests(message string) *AppError {
	if message == "" {
		message = "Too many requests"
	}
	return New(CodeTooManyRequests, message, http.StatusTooManyRequests)
}

func AsAppError(err error) *AppError {
	if err == nil {
		return nil
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.InvalidArgument:
			return BadRequest(st.Message())
		case codes.NotFound:
			return NotFound(st.Message())
		case codes.AlreadyExists:
			return Conflict(st.Message())
		case codes.Unauthenticated:
			return Unauthorized(st.Message())
		case codes.PermissionDenied:
			return Forbidden(st.Message())
		case codes.ResourceExhausted:
			return TooManyRequests(st.Message())
		case codes.Unavailable:
			return ServiceUnavailable(st.Message())
		case codes.Unknown:
			msg := st.Message()
			if strings.Contains(msg, "already exists") {
				return Conflict(msg)
			}
			if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") {
				return BadRequest(msg)
			}
			return Internal(err, msg)
		}
	}
	return Internal(err, err.Error())
}
