package middleware

import (
	"net/http"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/metrics"
)

func Metrics(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/metrics" || r.URL.Path == "/health" || r.URL.Path == "/ready" {
				next.ServeHTTP(w, r)
				return
			}

			metrics.IncHTTPInFlight(serviceName)
			defer metrics.DecHTTPInFlight(serviceName)

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
			metrics.RecordHTTPRequest(serviceName, r.Method, r.URL.Path, srw.StatusCode, duration)
		})
	}
}
