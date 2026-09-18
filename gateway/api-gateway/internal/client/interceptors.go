package client

import (
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"google.golang.org/grpc"
)

// UnaryClientInterceptor creates a gRPC client interceptor that propagates request context,
// metadata (Request ID, User ID, Role, Tracing), enforces timeout deadlines, and strips untrusted identity.
func UnaryClientInterceptor(defaultTimeout time.Duration) grpc.UnaryClientInterceptor {
	return grpcclient.UnaryClientInterceptor(defaultTimeout)
}
