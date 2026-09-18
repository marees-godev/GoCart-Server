package grpcclient

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	HeaderRequestID     = "x-request-id"
	HeaderCorrelationID = "x-correlation-id"
	HeaderUserID        = "x-user-id"
	HeaderUserRole      = "x-user-role"
	HeaderUserEmail     = "x-user-email"
)

// UnaryClientInterceptor creates a gRPC client interceptor that propagates request context,
// metadata (Request ID, User ID, Role, Tracing), enforces timeout deadlines, and strips untrusted identity.
func UnaryClientInterceptor(defaultTimeout time.Duration) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		} else {
			md = md.Copy()
		}

		// Strip client-supplied identity metadata to avoid spoofing
		md.Delete(HeaderUserID)
		md.Delete(HeaderUserRole)
		md.Delete(HeaderUserEmail)

		// Inject trusted identity from authenticated context
		if user, ok := auth.UserFromContext(ctx); ok && user != nil && user.UserID != "" {
			md.Set(HeaderUserID, user.UserID)
			if user.Role != "" {
				md.Set(HeaderUserRole, user.Role)
			}
			if user.Email != "" {
				md.Set(HeaderUserEmail, user.Email)
			}
		} else {
			if uID, ok := ctx.Value(logger.UserIDKey).(string); ok && uID != "" {
				md.Set(HeaderUserID, uID)
			} else if uID, ok := ctx.Value("userID").(string); ok && uID != "" {
				md.Set(HeaderUserID, uID)
			}
			if role, ok := ctx.Value("userRole").(string); ok && role != "" {
				md.Set(HeaderUserRole, role)
			}
		}

		// Propagate or generate Request ID
		reqID := middleware.GetRequestID(ctx)
		if reqID == "" {
			if id, ok := ctx.Value(logger.RequestIDKey).(string); ok && id != "" {
				reqID = id
			} else if id, ok := ctx.Value(logger.CorrelationIDKey).(string); ok && id != "" {
				reqID = id
			}
		}
		if reqID == "" {
			reqID = uuid.New().String()
		}
		md.Set(HeaderRequestID, reqID)
		md.Set(HeaderCorrelationID, reqID)

		tracingHeaders := make(map[string]string)
		tracing.InjectMessageContext(ctx, tracingHeaders)
		for k, v := range tracingHeaders {
			md.Set(k, v)
		}

		outCtx := metadata.NewOutgoingContext(ctx, md)

		// Enforce deadline/timeout if none set
		var cancel context.CancelFunc
		if _, hasDeadline := outCtx.Deadline(); !hasDeadline && defaultTimeout > 0 {
			outCtx, cancel = context.WithTimeout(outCtx, defaultTimeout)
			defer cancel()
		}

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

// UnaryServerInterceptor extracts propagated request metadata and initializes context for downstream services.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			var reqID string
			if ids := md.Get(HeaderRequestID); len(ids) > 0 && ids[0] != "" {
				reqID = ids[0]
			} else if cids := md.Get(HeaderCorrelationID); len(cids) > 0 && cids[0] != "" {
				reqID = cids[0]
			}
			if reqID == "" {
				reqID = uuid.New().String()
			}
			ctx = logger.WithRequestID(ctx, reqID)
			ctx = logger.WithCorrelationID(ctx, reqID)

			var userID, role, email string
			if uids := md.Get(HeaderUserID); len(uids) > 0 && uids[0] != "" {
				userID = uids[0]
			}
			if roles := md.Get(HeaderUserRole); len(roles) > 0 && roles[0] != "" {
				role = roles[0]
			}
			if emails := md.Get(HeaderUserEmail); len(emails) > 0 && emails[0] != "" {
				email = emails[0]
			}

			if userID != "" {
				ctx = logger.WithUserID(ctx, userID)
				ctx = context.WithValue(ctx, "userID", userID)
				ctx = context.WithValue(ctx, "userRole", role)
				userCtx := &auth.UserContext{
					UserID: userID,
					Role:   role,
					Email:  email,
				}
				ctx = auth.WithUser(ctx, userCtx)
			}

			tracingHeaders := make(map[string]string)
			for k, v := range md {
				if len(v) > 0 {
					tracingHeaders[k] = v[0]
				}
			}
			ctx = tracing.ExtractMessageContext(ctx, tracingHeaders)
		}

		return handler(ctx, req)
	}
}

// GetUserID extracts the propagated user ID from the context.
func GetUserID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if u, ok := auth.UserFromContext(ctx); ok && u != nil && u.UserID != "" {
		return u.UserID
	}
	if id, ok := ctx.Value(logger.UserIDKey).(string); ok && id != "" {
		return id
	}
	if id, ok := ctx.Value("userID").(string); ok && id != "" {
		return id
	}
	return ""
}

// GetUserRole extracts the propagated user role from the context.
func GetUserRole(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if u, ok := auth.UserFromContext(ctx); ok && u != nil && u.Role != "" {
		return u.Role
	}
	if role, ok := ctx.Value("userRole").(string); ok && role != "" {
		return role
	}
	return ""
}

// GetUserContext extracts the propagated UserContext.
func GetUserContext(ctx context.Context) (*auth.UserContext, bool) {
	return auth.UserFromContext(ctx)
}
