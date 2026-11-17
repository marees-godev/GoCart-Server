package maps

import (
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	personalizationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/personalization"
)

func MapProtoToGraphQLEventType(e personalizationpb.ActivityEventType) model.ActivityEventType {
	switch e {
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_VIEWED:
		return model.ActivityEventTypeProductViewed
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_SEARCHED:
		return model.ActivityEventTypeProductSearched
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_CATEGORY_VIEWED:
		return model.ActivityEventTypeCategoryViewed
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_ADDED_TO_CART:
		return model.ActivityEventTypeProductAddedToCart
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_REMOVED_FROM_CART:
		return model.ActivityEventTypeProductRemovedFromCart
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_WISHLISTED:
		return model.ActivityEventTypeProductWishlisted
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_PURCHASED:
		return model.ActivityEventTypeProductPurchased
	default:
		return model.ActivityEventTypeProductViewed
	}
}
