package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" || r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		var srw *StatusResponseWriter
		if existing, ok := w.(*StatusResponseWriter); ok {
			srw = existing
		} else {
			srw = NewStatusResponseWriter(w)
			w = srw
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

		slog.Log(ctx, level, "HTTP request completed",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", srw.StatusCode),
			slog.Float64("duration_ms", float64(duration.Microseconds())/1000.0),
			slog.Int64("bytes_written", srw.BytesWritten),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		)
	})
}
