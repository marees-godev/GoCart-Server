package handler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	personalizationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/personalization"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PersonalizationGRPCHandler struct {
	personalizationpb.UnimplementedPersonalizationServiceServer
	svc service.PersonalizationService
	log *slog.Logger
}

func NewPersonalizationGRPCHandler(svc service.PersonalizationService, log *slog.Logger) *PersonalizationGRPCHandler {
	if log == nil {
		log = slog.Default()
	}

	return &PersonalizationGRPCHandler{
		svc: svc,
		log: log,
	}
}

func (h *PersonalizationGRPCHandler) TrackActivity(ctx context.Context, req *personalizationpb.TrackActivityRequest) (*personalizationpb.TrackActivityResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	eventTypeStr, err := mapProtoToEventType(req.GetEventType())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	input := &dto.TrackActivityInput{
		UserID:     req.GetUserId(),
		SessionID:  req.GetSessionId(),
		EventType:  eventTypeStr,
		ProductID:  req.GetProductId(),
		CategoryID: req.GetCategoryId(),
	}

	if err := h.svc.TrackActivity(ctx, input); err != nil {
		h.log.ErrorContext(ctx, "Failed to track activity", "error", err)
		return nil, appErrors.ToGRPC(err)
	}

	return &personalizationpb.TrackActivityResponse{
		Success: true,
		Message: "Activity tracked successfully",
	}, nil
}

func (h *PersonalizationGRPCHandler) GetCustomerActivities(ctx context.Context, req *personalizationpb.GetCustomerActivitiesRequest) (*personalizationpb.GetCustomerActivitiesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	activities, total, err := h.svc.GetCustomerActivities(ctx, req.GetUserId(), req.GetSessionId(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		h.log.ErrorContext(ctx, "Failed to get customer activities", "error", err)
		return nil, appErrors.ToGRPC(err)
	}

	pbActivities := make([]*personalizationpb.CustomerActivity, len(activities))
	for i, act := range activities {
		pbActivities[i] = mapModelToProto(act)
	}

	return &personalizationpb.GetCustomerActivitiesResponse{
		Activities: pbActivities,
		Total:      int32(total),
	}, nil
}

func mapProtoToEventType(pbType personalizationpb.ActivityEventType) (string, error) {
	switch pbType {
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_VIEWED:
		return events.EventTypeProductViewed, nil
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_SEARCHED:
		return events.EventTypeProductSearched, nil
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_CATEGORY_VIEWED:
		return events.EventTypeCategoryViewed, nil
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_ADDED_TO_CART:
		return events.EventTypeProductAddedToCart, nil
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_REMOVED_FROM_CART:
		return events.EventTypeProductRemovedFromCart, nil
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_WISHLISTED:
		return events.EventTypeProductWishlisted, nil
	case personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_PURCHASED:
		return events.EventTypeProductPurchased, nil
	default:
		return "", fmt.Errorf("invalid activity event type: %v", pbType)
	}
}

func mapModelToProto(act *model.CustomerActivity) *personalizationpb.CustomerActivity {
	if act == nil {
		return nil
	}

	userID := ""
	if act.UserID != nil {
		userID = *act.UserID
	}

	sessionID := ""
	if act.SessionID != nil {
		sessionID = *act.SessionID
	}

	prodID := ""
	if act.ProductID != nil {
		prodID = *act.ProductID
	}

	catID := ""
	if act.CategoryID != nil {
		catID = *act.CategoryID
	}

	var eventType personalizationpb.ActivityEventType
	switch act.EventType {
	case "PRODUCT_VIEWED":
		eventType = personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_VIEWED
	case "PRODUCT_SEARCHED":
		eventType = personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_SEARCHED
	case "CATEGORY_VIEWED":
		eventType = personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_CATEGORY_VIEWED
	case "PRODUCT_ADDED_TO_CART":
		eventType = personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_ADDED_TO_CART
	case "PRODUCT_REMOVED_FROM_CART":
		eventType = personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_REMOVED_FROM_CART
	case "PRODUCT_WISHLISTED":
		eventType = personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_WISHLISTED
	case "PRODUCT_PURCHASED":
		eventType = personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_PRODUCT_PURCHASED
	default:
		eventType = personalizationpb.ActivityEventType_ACTIVITY_EVENT_TYPE_UNSPECIFIED
	}

	return &personalizationpb.CustomerActivity{
		Id:         act.ID,
		UserId:     userID,
		SessionId:  sessionID,
		EventType:  eventType,
		ProductId:  prodID,
		CategoryId: catID,
		CreatedAt:  act.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
