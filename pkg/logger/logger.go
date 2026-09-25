package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const (
	RequestIDKey     contextKey = "request_id"
	CorrelationIDKey contextKey = "correlation_id"
	UserIDKey        contextKey = "user_id"
	loggerContextKey contextKey = "slog_logger"
)

type Config struct {
	ServiceName   string
	Environment   string
	Version       string
	Level         string
	Format        string
	AddSource     bool
	DisableSource bool
	Output        io.Writer
}

func DefaultConfig(serviceName string) Config {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = os.Getenv("APP_ENV")
	}
	if env == "" {
		env = "development"
	}

	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "INFO"
	}

	format := os.Getenv("LOG_FORMAT")
	if format == "" {
		format = "json"
	}

	addSource := true
	if src := os.Getenv("LOG_ADD_SOURCE"); src != "" {
		if val, err := strconv.ParseBool(src); err == nil {
			addSource = val
		}
	}

	return Config{
		ServiceName:   serviceName,
		Environment:   env,
		Version:       os.Getenv("SERVICE_VERSION"),
		Level:         level,
		Format:        format,
		AddSource:     addSource,
		DisableSource: false,
		Output:        os.Stdout,
	}
}

type ContextHandler struct {
	handler slog.Handler
}

func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
			r.AddAttrs(slog.String("request_id", reqID))
		}
		if corrID, ok := ctx.Value(CorrelationIDKey).(string); ok && corrID != "" {
			r.AddAttrs(slog.String("correlation_id", corrID))
		}
		if userID, ok := ctx.Value(UserIDKey).(string); ok && userID != "" {
			r.AddAttrs(slog.String("user_id", userID))
		}

		span := trace.SpanFromContext(ctx)
		if span.SpanContext().IsValid() {
			r.AddAttrs(
				slog.String("trace_id", span.SpanContext().TraceID().String()),
				slog.String("span_id", span.SpanContext().SpanID().String()),
			)
		}
	}
	return h.handler.Handle(ctx, r)
}

func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{handler: h.handler.WithGroup(name)}
}

func New(cfg Config) *slog.Logger {
	var level slog.Level
	switch strings.ToUpper(cfg.Level) {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN", "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	addSource := true
	if cfg.DisableSource {
		addSource = false
	} else if envVal := os.Getenv("LOG_ADD_SOURCE"); envVal != "" {
		if parsed, err := strconv.ParseBool(envVal); err == nil {
			addSource = parsed
		}
	}
	if cfg.AddSource {
		addSource = true
	}

	format := strings.ToLower(cfg.Format)
	replaceAttr := func(groups []string, a slog.Attr) slog.Attr {
		a = RedactAttr(groups, a)
		if a.Key == slog.SourceKey {
			if source, ok := a.Value.Any().(*slog.Source); ok && source != nil {
				cleanPath := filepath.ToSlash(source.File)
				loc := fmt.Sprintf("%s:%d", cleanPath, source.Line)
				if format == "text" {
					return slog.String(slog.SourceKey, loc)
				}
				return slog.Group(slog.SourceKey,
					slog.String("location", loc),
					slog.String("file", loc),
					slog.Int("line", source.Line),
					slog.String("function", source.Function),
				)
			}
		}
		return a
	}

	opts := &slog.HandlerOptions{
		AddSource:   addSource,
		Level:       level,
		ReplaceAttr: replaceAttr,
	}

	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	var baseHandler slog.Handler
	if strings.ToLower(cfg.Format) == "text" {
		baseHandler = slog.NewTextHandler(out, opts)
	} else {
		baseHandler = slog.NewJSONHandler(out, opts)
	}

	var initialAttrs []slog.Attr
	if cfg.ServiceName != "" {
		initialAttrs = append(initialAttrs, slog.String("service", cfg.ServiceName))
	}
	if cfg.Environment != "" {
		initialAttrs = append(initialAttrs, slog.String("environment", cfg.Environment))
	}
	if cfg.Version != "" {
		initialAttrs = append(initialAttrs, slog.String("version", cfg.Version))
	}

	if len(initialAttrs) > 0 {
		baseHandler = baseHandler.WithAttrs(initialAttrs)
	}

	handler := &ContextHandler{handler: baseHandler}
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey, l)
}

func FromContext(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return slog.Default()
	}
	if l, ok := ctx.Value(loggerContextKey).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}
