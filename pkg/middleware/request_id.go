package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
)

const (
	HeaderXRequestID     = "X-Request-ID"
	HeaderXCorrelationID = "X-Correlation-ID"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get(HeaderXRequestID)
		if reqID == "" {
			reqID = r.Header.Get(HeaderXCorrelationID)
		}
		if reqID == "" {
			reqID = uuid.New().String()
		}

		ctx := logger.WithRequestID(r.Context(), reqID)
		ctx = logger.WithCorrelationID(ctx, reqID)

		w.Header().Set(HeaderXRequestID, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(logger.RequestIDKey).(string); ok {
		return id
	}
	return ""
}

