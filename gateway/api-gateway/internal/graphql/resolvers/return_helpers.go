package resolvers

import (
	"context"
	"strings"

	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	returnpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/return"
	storepb "github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) getStoreClient() (storepb.StoreServiceClient, error) {
	if r == nil {
		return nil, appErrors.Internal(nil, "store service client unavailable")
	}
	if r.Clients != nil && r.Clients.StoreClient != nil {
		return r.Clients.StoreClient, nil
	}
	if r.ClientMgr != nil && r.ClientMgr.StoreClient != nil {
		return r.ClientMgr.StoreClient, nil
	}
	return nil, appErrors.Internal(nil, "store service client unavailable")
}

func (r *queryResolver) merchantHasAccessToExchange(ctx context.Context, userCtx *auth.UserContext, exc *returnpb.Exchange) bool {
	if exc == nil || userCtx == nil {
		return false
	}
	merchantID := strings.TrimSpace(userCtx.MerchantID)
	if merchantID == "" {
		merchantID = strings.TrimSpace(userCtx.UserID)
	}
	if exc.UserId == merchantID || exc.UserId == userCtx.UserID {
		return true
	}

	prodID := strings.TrimSpace(exc.ProductId)
	if prodID != "" {
		productClient, err := r.getProductClient()
		if err == nil && productClient != nil {
			prodResp, err := productClient.GetProduct(ctx, &productpb.GetProductRequest{Id: prodID})
			if err == nil && prodResp != nil && prodResp.Product != nil {
				storeID := strings.TrimSpace(prodResp.Product.StoreId)
				if strings.EqualFold(storeID, merchantID) || strings.EqualFold(storeID, userCtx.UserID) {
					return true
				}
				storeClient, sErr := r.getStoreClient()
				if sErr == nil && storeClient != nil && storeID != "" {
					stResp, err := storeClient.GetStore(ctx, &storepb.GetStoreRequest{Id: storeID, MerchantId: merchantID})
					if err == nil && stResp != nil && stResp.Store != nil {
						if strings.EqualFold(stResp.Store.MerchantId, merchantID) || strings.EqualFold(stResp.Store.MerchantId, userCtx.UserID) {
							return true
						}
					}
				}
			}
		}
	}

	return false
}

func (r *queryResolver) deliveryPartnerHasAccessToExchange(_ context.Context, userCtx *auth.UserContext, exc *returnpb.Exchange) bool {
	if exc == nil || userCtx == nil {
		return false
	}
	partnerID := strings.TrimSpace(userCtx.PartnerID)
	if partnerID == "" {
		partnerID = strings.TrimSpace(userCtx.UserID)
	}
	if exc.UserId == partnerID || exc.UserId == userCtx.UserID {
		return true
	}

	if exc.Verification != nil {
		verifiedBy := strings.TrimSpace(exc.Verification.VerifiedBy)
		if verifiedBy != "" && (strings.EqualFold(verifiedBy, partnerID) || strings.EqualFold(verifiedBy, userCtx.UserID)) {
			return true
		}
	}

	return false
}