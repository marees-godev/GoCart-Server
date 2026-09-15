package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
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

				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":   "Internal Server Error",
					"message": fmt.Sprintf("%v", rec),
				})
			}
		}()

		next.ServeHTTP(w, r)
	})
}
