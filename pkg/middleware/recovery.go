package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/response"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				ctx := r.Context()
				stack := string(debug.Stack())

				slog.ErrorContext(ctx, "Panic recovered in HTTP handler",
					slog.Any("panic", rec),
					slog.String("stack", stack),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)

				response.Error(w, http.StatusInternalServerError, errors.CodeInternalError, "An internal server error occurred")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

