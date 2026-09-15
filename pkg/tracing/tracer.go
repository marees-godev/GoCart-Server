package tracing

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

type Config struct {
	ServiceName string
	Environment string
	Version     string
	Enabled     bool
	Exporter    string
}

func DefaultConfig(serviceName string) Config {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = os.Getenv("APP_ENV")
	}
	if env == "" {
		env = "development"
	}

	enabled := true
	if val := os.Getenv("TRACING_ENABLED"); strings.EqualFold(val, "false") || val == "0" {
		enabled = false
	}

	exporter := os.Getenv("TRACING_EXPORTER")
	if exporter == "" {
		if env == "production" || env == "staging" {
			exporter = "otlp"
		} else {
			exporter = "noop"
		}
	}

	return Config{
		ServiceName: serviceName,
		Environment: env,
		Version:     os.Getenv("SERVICE_VERSION"),
		Enabled:     enabled,
		Exporter:    exporter,
	}
}

func Init(cfg Config) (*sdktrace.TracerProvider, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if !cfg.Enabled || strings.EqualFold(cfg.Exporter, "noop") {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return nil, nil
	}

	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.DeploymentEnvironmentKey.String(cfg.Environment),
			semconv.ServiceVersionKey.String(cfg.Version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel resource: %w", err)
	}

	var exporter sdktrace.SpanExporter
	switch strings.ToLower(cfg.Exporter) {
	case "stdout":
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout trace exporter: %w", err)
		}
	default:
		otel.SetTracerProvider(noop.NewTracerProvider())
		return nil, nil
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	return tp, nil
}

func StartSpan(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tr := otel.GetTracerProvider().Tracer("gocart")
	return tr.Start(ctx, spanName, opts...)
}

func TraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

func SpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

func InjectHTTPHeaders(ctx context.Context, req *http.Request) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
}

func ExtractHTTPHeaders(req *http.Request) context.Context {
	return otel.GetTextMapPropagator().Extract(req.Context(), propagation.HeaderCarrier(req.Header))
}
