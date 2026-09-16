package middleware

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" || r.URL.Path == "/health" || r.URL.Path == "/ready" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		var srw *StatusResponseWriter
		if existing, ok := w.(*StatusResponseWriter); ok {
			srw = existing
		} else {
			srw = NewStatusResponseWriter(w)
		}

		next.ServeHTTP(srw, r)

		duration := time.Since(start)
		ctx := r.Context()

		level := slog.LevelInfo
		if srw.StatusCode >= 500 {
			level = slog.LevelError
		} else if srw.StatusCode >= 400 {
			level = slog.LevelWarn
		}

		attrs := []any{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", srw.StatusCode),
			slog.Float64("duration_ms", float64(duration.Microseconds())/1000.0),
			slog.Int64("bytes_written", srw.BytesWritten),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		}

		if r.URL.RawQuery != "" {
			attrs = append(attrs, slog.String("query", sanitizeQuery(r.URL.RawQuery)))
		}

		slog.Log(ctx, level, "HTTP request completed", attrs...)
	})
}

func sanitizeQuery(query string) string {
	if query == "" {
		return ""
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return "[REDACTED]"
	}
	for k := range values {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "password") || strings.Contains(lk, "token") ||
			strings.Contains(lk, "secret") || strings.Contains(lk, "key") ||
			strings.Contains(lk, "auth") || strings.Contains(lk, "cvv") ||
			strings.Contains(lk, "card") {
			values.Set(k, "[REDACTED]")
		}
	}
	return values.Encode()
}

