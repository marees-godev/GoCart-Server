package resolvers

import (
	"context"

	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
)

type Resolver struct {
	Clients *grpc.Clients
	Version string
}

func NewResolver(clients *grpc.Clients, version ...string) *Resolver {
	v := "1.0.0"
	if len(version) > 0 && version[0] != "" {
		v = version[0]
	}
	return &Resolver{
		Clients: clients,
		Version: v,
	}
}

func (r *Resolver) Health(ctx context.Context) (string, error) {
	return "OK", nil
}

func (r *Resolver) GetVersion(ctx context.Context) (string, error) {
	if r.Version == "" {
		return "1.0.0", nil
	}
	return r.Version, nil
}

func (r *Resolver) Me(ctx context.Context) (interface{}, error) {
	return r.Query().Me(ctx)
}

func (r *Resolver) User(ctx context.Context, id string) (interface{}, error) {
	return r.Query().User(ctx, id)
}

func getAuthUserIDFromCtx(ctx context.Context) string {
	if userCtx, ok := auth.UserFromContext(ctx); ok && userCtx != nil {
		return userCtx.UserID
	}
	if legacyID, ok := ctx.Value("userID").(string); ok {
		return legacyID
	}
	return ""
}

