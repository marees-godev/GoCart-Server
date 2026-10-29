package resolvers

import (
	"context"
	"strings"

	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
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

	orderID := strings.TrimSpace(exc.OrderId)
	if orderID == "" {
		return false
	}

	orderClient, err := r.getOrderClient()
	if err != nil || orderClient == nil {
		return false
	}

	ordResp, err := orderClient.GetOrder(ctx, &orderpb.GetOrderRequest{Id: orderID})
	if err != nil || ordResp == nil || ordResp.Order == nil {
		return false
	}

	ord := ordResp.Order
	if ord.UserId == merchantID || ord.UserId == userCtx.UserID {
		return true
	}

	storeIDs := make([]string, 0, 1+len(ord.Items))
	if s := strings.TrimSpace(ord.StoreId); s != "" {
		storeIDs = append(storeIDs, s)
	}

	var matchedItemStore string
	for _, it := range ord.Items {
		if it != nil && strings.TrimSpace(it.StoreId) != "" {
			if strings.TrimSpace(exc.OrderItemId) != "" && it.Id == exc.OrderItemId {
				matchedItemStore = strings.TrimSpace(it.StoreId)
			}
			storeIDs = append(storeIDs, strings.TrimSpace(it.StoreId))
		}
	}

	if matchedItemStore != "" {
		storeIDs = []string{matchedItemStore}
	}

	for _, sid := range storeIDs {
		if strings.EqualFold(sid, merchantID) || strings.EqualFold(sid, userCtx.UserID) {
			return true
		}
	}

	storeClient, sErr := r.getStoreClient()
	if sErr == nil && storeClient != nil {
		seen := make(map[string]bool)
		for _, sid := range storeIDs {
			if seen[sid] {
				continue
			}
			seen[sid] = true
			stResp, err := storeClient.GetStore(ctx, &storepb.GetStoreRequest{Id: sid, MerchantId: merchantID})
			if err == nil && stResp != nil && stResp.Store != nil {
				if strings.EqualFold(stResp.Store.MerchantId, merchantID) || strings.EqualFold(stResp.Store.MerchantId, userCtx.UserID) {
					return true
				}
			}
		}
	}

	return false
}

func (r *queryResolver) deliveryPartnerHasAccessToExchange(ctx context.Context, userCtx *auth.UserContext, exc *returnpb.Exchange) bool {
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

	orderID := strings.TrimSpace(exc.OrderId)
	if orderID == "" {
		return false
	}

	deliveryClient, err := r.getDeliveryClient()
	if err != nil || deliveryClient == nil {
		return false
	}

	delResp, err := deliveryClient.GetDeliveryByOrder(ctx, &deliverypb.GetDeliveryByOrderRequest{OrderId: orderID})
	if err != nil || delResp == nil || delResp.Delivery == nil {
		return false
	}

	assignedPartner := strings.TrimSpace(delResp.Delivery.DeliveryPartner)
	if assignedPartner != "" && (strings.EqualFold(assignedPartner, partnerID) || strings.EqualFold(assignedPartner, userCtx.UserID)) {
		return true
	}

	return false
}
