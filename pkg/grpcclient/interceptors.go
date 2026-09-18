package grpcclient

import (
	"context"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryClientInterceptor creates a gRPC client interceptor that propagates request context,
// metadata (Request ID, User ID, tracing), enforces timeout deadlines, and logs downstream errors.
func UnaryClientInterceptor(defaultTimeout time.Duration) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// 1. Inject outgoing metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		} else {
			md = md.Copy()
		}

		if reqID := middleware.GetRequestID(ctx); reqID != "" {
			md.Set("x-request-id", reqID)
		}

		if userID, ok := ctx.Value(logger.UserIDKey).(string); ok && userID != "" {
			md.Set("x-user-id", userID)
		} else if uID, ok := ctx.Value("userID").(string); ok && uID != "" {
			md.Set("x-user-id", uID)
		}

		tracingHeaders := make(map[string]string)
		tracing.InjectMessageContext(ctx, tracingHeaders)
		for k, v := range tracingHeaders {
			md.Set(k, v)
		}

		outCtx := metadata.NewOutgoingContext(ctx, md)

		// 2. Enforce deadline/timeout if none set
		var cancel context.CancelFunc
		if _, hasDeadline := outCtx.Deadline(); !hasDeadline && defaultTimeout > 0 {
			outCtx, cancel = context.WithTimeout(outCtx, defaultTimeout)
			defer cancel()
		}

		// 3. Invoke downstream RPC
		start := time.Now()
		err := invoker(outCtx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		if err != nil {
			logger.FromContext(ctx).Warn("Service-to-service gRPC call failed",
				"method", method,
				"target", cc.Target(),
				"duration", duration.String(),
				"error", err,
			)
		}

		return err
	}
}
