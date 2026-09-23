package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type MapCarrier map[string]string

var _ propagation.TextMapCarrier = MapCarrier{}

func (c MapCarrier) Get(key string) string {
	return c[key]
}

func (c MapCarrier) Set(key string, value string) {
	c[key] = value
}

func (c MapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

func InjectMessageContext(ctx context.Context, headers map[string]string) {
	if headers == nil {
		return
	}
	carrier := MapCarrier(headers)
	otel.GetTextMapPropagator().Inject(ctx, carrier)
}

func ExtractMessageContext(ctx context.Context, headers map[string]string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(headers) == 0 {
		return ctx
	}
	carrier := MapCarrier(headers)
	resCtx := otel.GetTextMapPropagator().Extract(ctx, carrier)
	if resCtx == nil {
		return ctx
	}
	return resCtx
}

type ByteMapCarrier map[string][]byte

func (c ByteMapCarrier) Get(key string) string {
	if val, ok := c[key]; ok {
		return string(val)
	}
	return ""
}

func (c ByteMapCarrier) Set(key string, value string) {
	c[key] = []byte(value)
}

func (c ByteMapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

func InjectByteMessageContext(ctx context.Context, headers map[string][]byte) {
	if headers == nil {
		return
	}
	carrier := ByteMapCarrier(headers)
	otel.GetTextMapPropagator().Inject(ctx, carrier)
}

func ExtractByteMessageContext(ctx context.Context, headers map[string][]byte) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(headers) == 0 {
		return ctx
	}
	carrier := ByteMapCarrier(headers)
	resCtx := otel.GetTextMapPropagator().Extract(ctx, carrier)
	if resCtx == nil {
		return ctx
	}
	return resCtx
}
