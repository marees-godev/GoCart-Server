package response

import (
	"encoding/json"
	"net/http"

	"github.com/marees-godev/GoCart-Server/pkg/errors"
)

type ErrorDetail = errors.ErrorDetail
type ErrorResponse = errors.ErrorResponse

func WriteJSON(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if data == nil {
		return nil
	}
	return json.NewEncoder(w).Encode(data)
}

func Error(w http.ResponseWriter, status int, code, message string) *ErrorResponse {
	if code == "" {
		code = errors.CodeInternalError
	}
	if message == "" {
		message = "An internal server error occurred"
	}

	resp := errors.NewErrorResponse(code, message)
	_ = WriteJSON(w, status, resp)
	return resp
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) *ErrorResponse {
	if err == nil {
		return nil
	}

	appErr := errors.AsAppError(err)
	status := appErr.HTTPStatus
	if status == 0 {
		status = http.StatusInternalServerError
	}

	resp := appErr.ToResponse()
	_ = WriteJSON(w, status, resp)
	return resp
}
