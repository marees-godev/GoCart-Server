package resolver

import (
	"context"
)

type Resolver struct {
	Version string
}

func NewResolver(version string) *Resolver {
	if version == "" {
		version = "1.0.0"
	}
	return &Resolver{
		Version: version,
	}
}

func (r *Resolver) Health(ctx context.Context) (string, error) {
	return "OK", nil
}

func (r *Resolver) GetVersion(ctx context.Context) (string, error) {
	return r.Version, nil
}
