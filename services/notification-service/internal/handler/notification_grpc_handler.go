package handler

import (
	"context"
	"strings"
	"time"

	notificationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/notification"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
)

type NotificationGRPCHandler struct {
	notificationpb.UnimplementedNotificationServiceServer
	notifSvc service.NotificationService
	subSvc   service.PushSubscriptionService
	prefSvc  service.PreferenceService
}

func NewNotificationGRPCHandler(
	notifSvc service.NotificationService,
	subSvc service.PushSubscriptionService,
	prefSvc service.PreferenceService,
) *NotificationGRPCHandler {
	return &NotificationGRPCHandler{
		notifSvc: notifSvc,
		subSvc:   subSvc,
		prefSvc:  prefSvc,
	}
}

func (h *NotificationGRPCHandler) resolveAuthorizedUserID(ctx context.Context, requestedUserID string) (string, error) {
	reqUID := strings.TrimSpace(requestedUserID)
	callerUID := strings.TrimSpace(grpcclient.GetUserID(ctx))
	callerRole := strings.ToUpper(strings.TrimSpace(grpcclient.GetUserRole(ctx)))

	if callerUID == "" || callerRole == "ADMIN" || callerRole == "MERCHANT" || callerRole == "DELIVERY_PARTNER" || callerRole == "SYSTEM" {
		if reqUID != "" {
			return reqUID, nil
		}
		return callerUID, nil
	}
	if reqUID != "" && reqUID != callerUID {
		return "", appErrors.Forbidden("access denied: cannot operate on another user's notifications or push subscriptions")
	}
	return callerUID, nil
}

func (h *NotificationGRPCHandler) SendNotification(
	ctx context.Context,
	req *notificationpb.SendNotificationRequest,
) (*notificationpb.SendNotificationResponse, error) {
	if req == nil {
		return nil, appErrors.ToGRPC(appErrors.BadRequest("request cannot be nil"))
	}

	userID, err := h.resolveAuthorizedUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	correlationID := strings.TrimSpace(req.GetCorrelationId())
	if correlationID == "" && ctx != nil {
		if cid, ok := ctx.Value(logger.CorrelationIDKey).(string); ok && cid != "" {
			correlationID = cid
		} else if rid, ok := ctx.Value(logger.RequestIDKey).(string); ok && rid != "" {
			correlationID = rid
		}
	}

	eventID := ""
	if req.GetTemplateData() != nil {
		if eid, ok := req.GetTemplateData()["event_id"]; ok && strings.TrimSpace(eid) != "" {
			eventID = strings.TrimSpace(eid)
		}
	}
	if eventID == "" {
		eventID = correlationID
	}

	res, err := h.notifSvc.SendNotification(ctx, dto.SendNotificationInput{
		UserID:        userID,
		Channel:       req.GetChannel(),
		EventType:     req.GetEventType(),
		EventID:       eventID,
		Recipient:     req.GetRecipient(),
		Subject:       req.GetSubject(),
		Content:       req.GetContent(),
		TemplateID:    req.GetTemplateId(),
		TemplateData:  req.GetTemplateData(),
		CorrelationID: correlationID,
	})
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	resp := &notificationpb.SendNotificationResponse{
		Success: res.Success,
		Message: res.Message,
	}
	if res.Notification != nil {
		resp.NotificationId = res.Notification.ID
		resp.Status = string(res.Notification.Status)
	}
	return resp, nil
}

func (h *NotificationGRPCHandler) GetNotifications(
	ctx context.Context,
	req *notificationpb.GetNotificationsRequest,
) (*notificationpb.GetNotificationsResponse, error) {
	if req == nil {
		return nil, appErrors.ToGRPC(appErrors.BadRequest("request cannot be nil"))
	}

	userID, err := h.resolveAuthorizedUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	var isRead *bool
	if req.IsRead != nil {
		v := req.GetIsRead()
		isRead = &v
	}

	res, err := h.notifSvc.GetNotifications(ctx, model.NotificationFilter{
		UserID:    userID,
		Status:    req.GetStatus(),
		Channel:   req.GetChannel(),
		EventType: req.GetEventType(),
		IsRead:    isRead,
		Limit:     int(req.GetLimit()),
		Offset:    int(req.GetOffset()),
	})
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	protoNotifs := make([]*notificationpb.Notification, 0, len(res.Notifications))
	for _, n := range res.Notifications {
		protoNotifs = append(protoNotifs, mapModelNotificationToProto(n))
	}

	return &notificationpb.GetNotificationsResponse{
		Notifications: protoNotifs,
		Total:         int32(res.Total),
		UnreadCount:   int32(res.UnreadCount),
	}, nil
}

func (h *NotificationGRPCHandler) MarkNotificationAsRead(
	ctx context.Context,
	req *notificationpb.MarkNotificationAsReadRequest,
) (*notificationpb.MarkNotificationAsReadResponse, error) {
	if req == nil {
		return nil, appErrors.ToGRPC(appErrors.BadRequest("request cannot be nil"))
	}

	userID, err := h.resolveAuthorizedUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	isRead := true
	if req.IsRead != nil {
		isRead = req.GetIsRead()
	}

	updated, err := h.notifSvc.MarkAsRead(ctx, req.GetNotificationId(), userID, isRead)
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &notificationpb.MarkNotificationAsReadResponse{
		Notification: mapModelNotificationToProto(updated),
		Success:      true,
	}, nil
}

func (h *NotificationGRPCHandler) MarkAllNotificationsAsRead(
	ctx context.Context,
	req *notificationpb.MarkAllNotificationsAsReadRequest,
) (*notificationpb.MarkAllNotificationsAsReadResponse, error) {
	if req == nil {
		return nil, appErrors.ToGRPC(appErrors.BadRequest("request cannot be nil"))
	}

	userID, err := h.resolveAuthorizedUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	updatedCount, err := h.notifSvc.MarkAllAsRead(ctx, userID)
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &notificationpb.MarkAllNotificationsAsReadResponse{
		UpdatedCount: int32(updatedCount),
		Success:      true,
	}, nil
}

func (h *NotificationGRPCHandler) GetUnreadCount(
	ctx context.Context,
	req *notificationpb.GetUnreadCountRequest,
) (*notificationpb.GetUnreadCountResponse, error) {
	if req == nil {
		return nil, appErrors.ToGRPC(appErrors.BadRequest("request cannot be nil"))
	}

	userID, err := h.resolveAuthorizedUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	count, err := h.notifSvc.GetUnreadCount(ctx, userID)
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &notificationpb.GetUnreadCountResponse{
		UnreadCount: int32(count),
	}, nil
}

func (h *NotificationGRPCHandler) RegisterPushSubscription(
	ctx context.Context,
	req *notificationpb.RegisterPushSubscriptionRequest,
) (*notificationpb.PushSubscriptionResponse, error) {
	if req == nil {
		return nil, appErrors.ToGRPC(appErrors.BadRequest("request cannot be nil"))
	}

	userID, err := h.resolveAuthorizedUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	saved, err := h.subSvc.Register(ctx, dto.RegisterPushSubscriptionInput{
		UserID:     userID,
		Endpoint:   req.GetEndpoint(),
		P256DH:     req.GetP256Dh(),
		Auth:       req.GetAuth(),
		UserAgent:  req.GetUserAgent(),
		DeviceName: req.GetDeviceName(),
	})
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &notificationpb.PushSubscriptionResponse{
		Subscription: mapModelPushSubscriptionToProto(saved),
	}, nil
}

func (h *NotificationGRPCHandler) GetPushSubscriptions(
	ctx context.Context,
	req *notificationpb.GetPushSubscriptionsRequest,
) (*notificationpb.GetPushSubscriptionsResponse, error) {
	if req == nil {
		return nil, appErrors.ToGRPC(appErrors.BadRequest("request cannot be nil"))
	}

	userID, err := h.resolveAuthorizedUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	subs, err := h.subSvc.GetByUserID(ctx, userID, req.GetActiveOnly())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	protoSubs := make([]*notificationpb.PushSubscription, 0, len(subs))
	for _, s := range subs {
		protoSubs = append(protoSubs, mapModelPushSubscriptionToProto(s))
	}

	return &notificationpb.GetPushSubscriptionsResponse{
		Subscriptions: protoSubs,
		Total:         int32(len(protoSubs)),
	}, nil
}

func (h *NotificationGRPCHandler) UpdatePushSubscription(
	ctx context.Context,
	req *notificationpb.UpdatePushSubscriptionRequest,
) (*notificationpb.PushSubscriptionResponse, error) {
	if req == nil {
		return nil, appErrors.ToGRPC(appErrors.BadRequest("request cannot be nil"))
	}

	userID, err := h.resolveAuthorizedUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	var isActive *bool
	if req.IsActive != nil {
		v := req.GetIsActive()
		isActive = &v
	}

	updated, err := h.subSvc.Update(ctx, dto.UpdatePushSubscriptionInput{
		ID:         req.GetId(),
		UserID:     userID,
		Endpoint:   req.GetEndpoint(),
		P256DH:     req.GetP256Dh(),
		Auth:       req.GetAuth(),
		UserAgent:  req.GetUserAgent(),
		DeviceName: req.GetDeviceName(),
		IsActive:   isActive,
	})
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &notificationpb.PushSubscriptionResponse{
		Subscription: mapModelPushSubscriptionToProto(updated),
	}, nil
}

func (h *NotificationGRPCHandler) DeactivatePushSubscription(
	ctx context.Context,
	req *notificationpb.DeactivatePushSubscriptionRequest,
) (*notificationpb.DeactivatePushSubscriptionResponse, error) {
	if req == nil {
		return nil, appErrors.ToGRPC(appErrors.BadRequest("request cannot be nil"))
	}

	userID, err := h.resolveAuthorizedUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	deactivated, err := h.subSvc.Deactivate(ctx, dto.DeactivatePushSubscriptionInput{
		ID:       req.GetId(),
		UserID:   userID,
		Endpoint: req.GetEndpoint(),
	})
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	msg := "push subscription deactivated"
	if !deactivated {
		msg = "push subscription already inactive"
	}

	return &notificationpb.DeactivatePushSubscriptionResponse{
		Success: deactivated,
		Message: msg,
	}, nil
}

func (h *NotificationGRPCHandler) GetVAPIDPublicKey(
	ctx context.Context,
	req *notificationpb.GetVAPIDPublicKeyRequest,
) (*notificationpb.GetVAPIDPublicKeyResponse, error) {
	pubKey, subject, err := h.subSvc.GetVAPIDPublicKey(ctx)
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}
	return &notificationpb.GetVAPIDPublicKeyResponse{
		PublicKey: pubKey,
		Subject:   subject,
	}, nil
}

func (h *NotificationGRPCHandler) GetUserPreferences(ctx context.Context, req *notificationpb.GetUserPreferencesRequest) (*notificationpb.GetUserPreferencesResponse, error) {
	if req == nil {
		return nil, appErrors.ToGRPC(appErrors.BadRequest("request cannot be nil"))
	}

	userID, err := h.resolveAuthorizedUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	prefs, err := h.prefSvc.GetUserPreferences(ctx, userID)
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	items := make([]*notificationpb.PreferenceItem, 0, len(prefs))
	for _, p := range prefs {
		items = append(items, &notificationpb.PreferenceItem{
			Channel:   p.ChannelID,
			Category:  p.Category,
			IsEnabled: p.IsEnabled,
		})
	}

	return &notificationpb.GetUserPreferencesResponse{
		Preferences: items,
	}, nil
}

func (h *NotificationGRPCHandler) UpdateUserPreference(ctx context.Context, req *notificationpb.UpdateUserPreferenceRequest) (*notificationpb.UpdateUserPreferenceResponse, error) {
	if req == nil {
		return nil, appErrors.ToGRPC(appErrors.BadRequest("request cannot be nil"))
	}

	userID, err := h.resolveAuthorizedUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	pref, err := h.prefSvc.UpdateUserPreference(ctx, userID, req.GetChannel(), req.GetCategory(), req.GetIsEnabled())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &notificationpb.UpdateUserPreferenceResponse{
		Success: true,
		Preference: &notificationpb.PreferenceItem{
			Channel:   req.GetChannel(),
			Category:  pref.Category,
			IsEnabled: pref.IsEnabled,
		},
	}, nil
}

func mapModelNotificationToProto(n *model.Notification) *notificationpb.Notification {
	if n == nil {
		return nil
	}
	channel := n.ChannelCode
	if channel == "" {
		channel = n.ChannelID
	}
	eventID := ""
	if n.EventID != nil {
		eventID = *n.EventID
	}
	errMsg := ""
	if n.ErrorMessage != nil {
		errMsg = *n.ErrorMessage
	}
	readAt := ""
	if n.ReadAt != nil {
		readAt = n.ReadAt.UTC().Format(time.RFC3339)
	}
	sentAt := ""
	if n.SentAt != nil {
		sentAt = n.SentAt.UTC().Format(time.RFC3339)
	}

	return &notificationpb.Notification{
		Id:            n.ID,
		UserId:        n.UserID,
		Channel:       channel,
		Subject:       n.Title,
		Content:       n.Content,
		Status:        string(n.Status),
		CreatedAt:     n.CreatedAt.UTC().Format(time.RFC3339),
		EventType:     n.EventType,
		Recipient:     n.Recipient,
		ErrorMessage:  errMsg,
		IsRead:        n.IsRead,
		ReadAt:        readAt,
		SentAt:        sentAt,
		CorrelationId: n.CorrelationID,
		EventId:       eventID,
		UpdatedAt:     n.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func mapModelPushSubscriptionToProto(sub *model.PushSubscription) *notificationpb.PushSubscription {
	if sub == nil {
		return nil
	}
	lastErr := ""
	if sub.LastError != nil {
		lastErr = *sub.LastError
	}
	lastUsedAt := ""
	if sub.LastUsedAt != nil {
		lastUsedAt = sub.LastUsedAt.UTC().Format(time.RFC3339)
	}
	deactivatedAt := ""
	if sub.DeactivatedAt != nil {
		deactivatedAt = sub.DeactivatedAt.UTC().Format(time.RFC3339)
	}

	return &notificationpb.PushSubscription{
		Id:            sub.ID,
		UserId:        sub.UserID,
		Endpoint:      sub.Endpoint,
		UserAgent:     sub.UserAgent,
		DeviceName:    sub.DeviceName,
		IsActive:      sub.IsActive,
		FailureCount:  int32(sub.FailureCount),
		LastError:     lastErr,
		LastUsedAt:    lastUsedAt,
		DeactivatedAt: deactivatedAt,
		CreatedAt:     sub.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     sub.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
