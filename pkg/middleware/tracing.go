package middleware

import (
	"fmt"
	"net/http"

	"github.com/marees-godev/GoCart-Server/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

func Tracing(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := tracing.ExtractHTTPHeaders(r)

			spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
			ctx, span := tracing.StartSpan(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPRequestMethodKey.String(r.Method),
					semconv.URLPath(r.URL.Path),
					semconv.UserAgentOriginal(r.UserAgent()),
					attribute.String("service", serviceName),
				),
			)
			defer span.End()

			srw := NewStatusResponseWriter(w)
			next.ServeHTTP(srw, r.WithContext(ctx))

			span.SetAttributes(semconv.HTTPResponseStatusCode(srw.StatusCode))
			if srw.StatusCode >= 500 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", srw.StatusCode))
			} else {
				span.SetStatus(codes.Ok, "OK")
			}
		})
	}
}
