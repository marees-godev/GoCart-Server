package resolvers

import (
	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) getAuthClient() (authpb.AuthServiceClient, error) {
	if r == nil {
		return nil, appErrors.Internal(nil, "auth client unavailable")
	}
	if r.Clients != nil && r.Clients.AuthClient != nil {
		return r.Clients.AuthClient, nil
	}
	if r.ClientMgr != nil && r.ClientMgr.AuthClient != nil {
		return r.ClientMgr.AuthClient, nil
	}
	return nil, appErrors.Internal(nil, "auth client unavailable")
}

func (r *mutationResolver) getAuthClient() (authpb.AuthServiceClient, error) {
	if r == nil || r.Resolver == nil {
		return nil, appErrors.Internal(nil, "auth client unavailable")
	}
	return r.Resolver.getAuthClient()
}
