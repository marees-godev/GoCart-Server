package resolvers

import (
	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
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

func (r *Resolver) getDeliveryClient() (deliverypb.DeliveryServiceClient, error) {
	if r == nil {
		return nil, appErrors.ServiceUnavailable("delivery service unavailable")
	}
	if r.Clients != nil && r.Clients.DeliveryClient != nil {
		return r.Clients.DeliveryClient, nil
	}
	if r.ClientMgr != nil && r.ClientMgr.DeliveryClient != nil {
		return r.ClientMgr.DeliveryClient, nil
	}
	return nil, appErrors.ServiceUnavailable("delivery service unavailable")
}

func (r *mutationResolver) getDeliveryClient() (deliverypb.DeliveryServiceClient, error) {
	if r == nil || r.Resolver == nil {
		return nil, appErrors.ServiceUnavailable("delivery service unavailable")
	}
	return r.Resolver.getDeliveryClient()
}
