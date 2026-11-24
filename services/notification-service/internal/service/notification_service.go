package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/push"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
)

type NotificationService interface {
	SendNotification(ctx context.Context, input dto.SendNotificationInput) (*dto.SendNotificationResult, error)
	GetNotifications(ctx context.Context, filter model.NotificationFilter) (*dto.ListNotificationsResult, error)
	MarkAsRead(ctx context.Context, notificationID, userID string, isRead bool) (*model.Notification, error)
	MarkAllAsRead(ctx context.Context, userID string) (int, error)
	GetUnreadCount(ctx context.Context, userID string) (int, error)
}

type notificationService struct {
	channelRepo     repository.ChannelRepository
	prefRepo        repository.PreferenceRepository
	historyRepo     repository.NotificationHistoryRepository
	subRepo         repository.PushSubscriptionRepository
	templateService TemplateService
	pushSender      push.Sender
	localNotifier   push.LocalNotifier
	logger          *slog.Logger
}

func NewNotificationService(
	channelRepo repository.ChannelRepository,
	prefRepo repository.PreferenceRepository,
	historyRepo repository.NotificationHistoryRepository,
	subRepo repository.PushSubscriptionRepository,
	templateService TemplateService,
	pushSender push.Sender,
	logger *slog.Logger,
	localNotifier ...push.LocalNotifier,
) NotificationService {
	if logger == nil {
		logger = slog.Default()
	}
	var ln push.LocalNotifier
	if len(localNotifier) > 0 && localNotifier[0] != nil {
		ln = localNotifier[0]
	} else {
		ln = push.NewLocalSystemNotifier(logger, true)
	}
	return &notificationService{
		channelRepo:     channelRepo,
		prefRepo:        prefRepo,
		historyRepo:     historyRepo,
		subRepo:         subRepo,
		templateService: templateService,
		pushSender:      pushSender,
		localNotifier:   ln,
		logger:          logger,
	}
}

func (s *notificationService) SendNotification(ctx context.Context, input dto.SendNotificationInput) (*dto.SendNotificationResult, error) {
	userID := strings.TrimSpace(input.UserID)
	if _, err := uuid.Parse(userID); err != nil {
		return nil, appErrors.BadRequest("valid user_id is required")
	}

	channelCode := strings.ToUpper(strings.TrimSpace(input.Channel))
	if channelCode == "" {
		channelCode = model.ChannelPush
	}

	channel, err := s.channelRepo.GetByCode(ctx, channelCode)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to resolve notification channel")
	}
	if channel == nil {
		return nil, appErrors.BadRequest(fmt.Sprintf("unsupported notification channel: %s", channelCode))
	}

	eventID := strings.TrimSpace(input.EventID)
	if eventID != "" {
		existing, err := s.historyRepo.GetNotificationByEventIDAndChannel(ctx, eventID, channel.ID)
		if err != nil {
			return nil, appErrors.Internal(err, "failed checking duplicate notification")
		}
		if existing != nil {
			return &dto.SendNotificationResult{
				Notification: existing,
				Success:      true,
				Message:      "notification already exists for event",
			}, nil
		}
	}

	title := strings.TrimSpace(input.Subject)
	content := strings.TrimSpace(input.Content)
	eventType := strings.TrimSpace(input.EventType)
	if eventType == "" {
		eventType = "DirectNotification"
	}

	if (title == "" || content == "") && s.templateService != nil && eventType != "" {
		if tmpl, tErr := s.templateService.ResolveTemplateForChannel(ctx, eventType, channel.ID, channel.Code); tErr == nil && tmpl != nil {
			dataMap := make(map[string]interface{}, len(input.TemplateData))
			for k, v := range input.TemplateData {
				dataMap[k] = v
			}
			renderedSubj, renderedBody, rErr := s.templateService.Render(tmpl, dataMap)
			if rErr == nil {
				if title == "" {
					title = renderedSubj
				}
				if content == "" {
					content = renderedBody
				}
			}
		}
	}

	if title == "" || content == "" {
		return nil, appErrors.BadRequest("subject and content are required")
	}

	recipient := strings.TrimSpace(input.Recipient)
	if recipient == "" {
		recipient = userID
	}

	var eventIDPtr *string
	if eventID != "" {
		eventIDPtr = &eventID
	}

	notif := &model.Notification{
		ID:            uuid.NewString(),
		UserID:        userID,
		ChannelID:     channel.ID,
		ChannelCode:   channel.Code,
		EventType:     truncateString(eventType, 100),
		EventID:       eventIDPtr,
		Recipient:     truncateString(recipient, 255),
		Title:         truncateString(title, 255),
		Content:       content,
		Status:        model.DeliveryStatusPending,
		CorrelationID: strings.TrimSpace(input.CorrelationID),
	}

	if err := s.historyRepo.CreateNotification(ctx, notif); err != nil {
		if strings.Contains(err.Error(), "already exists") && eventID != "" {
			existing, getErr := s.historyRepo.GetNotificationByEventIDAndChannel(ctx, eventID, channel.ID)
			if getErr == nil && existing != nil {
				return &dto.SendNotificationResult{
					Notification: existing,
					Success:      true,
					Message:      "notification already exists for event",
				}, nil
			}
		}
		return nil, appErrors.Internal(err, "failed to persist notification")
	}

	if channel.Code != model.ChannelPush {
		if channel.Code == model.ChannelInApp {
			_ = s.historyRepo.UpdateDeliveryStatus(ctx, notif.ID, model.DeliveryStatusSent, nil, false)
			now := time.Now().UTC()
			notif.Status = model.DeliveryStatusSent
			notif.SentAt = &now
		}
		return &dto.SendNotificationResult{
			Notification: notif,
			Success:      true,
			Message:      fmt.Sprintf("%s notification recorded", channel.Code),
		}, nil
	}

	if s.prefRepo != nil && eventType != "" {
		if !isCriticalNotification(eventType) {
			category := EventToCategory(eventType)
			pref, prefErr := s.prefRepo.GetPreference(ctx, userID, channel.ID, category)
			if prefErr != nil {
				s.logger.WarnContext(ctx, "Failed to check push preference; proceeding with default allow",
					"user_id", userID,
					"event_type", eventType,
					"category", category,
					"error", prefErr,
				)
			} else if pref != nil && !pref.IsEnabled {
				s.logger.InfoContext(ctx, "User opted out of PUSH notifications for category; history preserved without push",
					"notification_id", notif.ID,
					"user_id", userID,
					"category", category,
				)
				return &dto.SendNotificationResult{
					Notification: notif,
					Success:      true,
					Message:      "notification saved; user opted out of push delivery",
				}, nil
			}
		}
	}

	return s.deliverPush(ctx, notif, input.TemplateData), nil
}

func (s *notificationService) deliverPush(
	ctx context.Context,
	notif *model.Notification,
	data map[string]string,
) *dto.SendNotificationResult {
	if s.localNotifier != nil {
		s.localNotifier.NotifyAsync(notif.Content)
	}

	if s.subRepo == nil {
		return &dto.SendNotificationResult{
			Notification: notif,
			Success:      true,
			Message:      "notification saved; push subscription repository unavailable",
		}
	}

	subs, err := s.subRepo.ListSubscriptionsByUserID(ctx, notif.UserID, true)
	if err != nil {
		errMsg := "failed retrieving active push subscriptions"
		_ = s.historyRepo.UpdateDeliveryStatus(ctx, notif.ID, model.DeliveryStatusFailed, &errMsg, false)
		notif.Status = model.DeliveryStatusFailed
		notif.ErrorMessage = &errMsg
		return &dto.SendNotificationResult{
			Notification: notif,
			Success:      false,
			Message:      errMsg,
		}
	}

	if len(subs) == 0 {
		s.logger.InfoContext(ctx, "No active push subscriptions for user; skipping push delivery gracefully",
			"notification_id", notif.ID,
			"user_id", notif.UserID,
			"event_type", notif.EventType,
			"correlation_id", notif.CorrelationID,
		)
		return &dto.SendNotificationResult{
			Notification: notif,
			Success:      true,
			Message:      "notification saved; no active push subscriptions registered",
		}
	}

	if s.pushSender == nil || !s.pushSender.IsConfigured() {
		errMsg := push.ErrVAPIDNotConfigured.Error()
		_ = s.historyRepo.UpdateDeliveryStatus(ctx, notif.ID, model.DeliveryStatusFailed, &errMsg, false)
		notif.Status = model.DeliveryStatusFailed
		notif.ErrorMessage = &errMsg
		s.logger.WarnContext(ctx, "Web Push skipped because VAPID keys are not configured",
			"notification_id", notif.ID,
			"user_id", notif.UserID,
		)
		return &dto.SendNotificationResult{
			Notification: notif,
			Success:      false,
			Message:      errMsg,
		}
	}

	payloadObj := dto.PushPayload{
		NotificationID: notif.ID,
		Title:          notif.Title,
		Body:           notif.Content,
		EventType:      notif.EventType,
		CorrelationID:  notif.CorrelationID,
		Timestamp:      notif.CreatedAt.UTC().Format(time.RFC3339),
		Data:           data,
	}
	payloadBytes, err := json.Marshal(payloadObj)
	if err != nil {
		errMsg := "failed serializing push payload"
		_ = s.historyRepo.UpdateDeliveryStatus(ctx, notif.ID, model.DeliveryStatusFailed, &errMsg, false)
		notif.Status = model.DeliveryStatusFailed
		notif.ErrorMessage = &errMsg
		return &dto.SendNotificationResult{
			Notification: notif,
			Success:      false,
			Message:      errMsg,
		}
	}

	var (
		sentCount       int
		hadRetryableErr bool
		lastErrMsg      string
	)

	for _, sub := range subs {
		if sub == nil || !sub.IsActive {
			continue
		}
		res, sendErr := s.pushSender.Send(ctx, sub, payloadBytes)
		if sendErr == nil {
			sentCount++
			_ = s.subRepo.MarkSubscriptionUsed(ctx, sub.ID)
			s.logger.InfoContext(ctx, "Web Push notification delivered",
				"notification_id", notif.ID,
				"subscription_id", sub.ID,
				"user_id", notif.UserID,
				"correlation_id", notif.CorrelationID,
			)
			continue
		}

		lastErrMsg = sendErr.Error()
		if errors.Is(sendErr, push.ErrSubscriptionGone) || (res != nil && res.SubscriptionGone) {
			_ = s.subRepo.MarkSubscriptionGone(ctx, sub.ID, lastErrMsg)
			s.logger.WarnContext(ctx, "Deactivated expired push subscription (404/410)",
				"notification_id", notif.ID,
				"subscription_id", sub.ID,
				"user_id", notif.UserID,
				"error", lastErrMsg,
			)
			continue
		}

		if res != nil && res.Retryable {
			hadRetryableErr = true
		}
		_ = s.subRepo.RecordSubscriptionFailure(ctx, sub.ID, lastErrMsg)
		s.logger.WarnContext(ctx, "Web Push delivery failed for subscription",
			"notification_id", notif.ID,
			"subscription_id", sub.ID,
			"user_id", notif.UserID,
			"correlation_id", notif.CorrelationID,
			"error", lastErrMsg,
		)
	}

	if sentCount > 0 {
		_ = s.historyRepo.UpdateDeliveryStatus(ctx, notif.ID, model.DeliveryStatusSent, nil, false)
		now := time.Now().UTC()
		notif.Status = model.DeliveryStatusSent
		notif.SentAt = &now
		notif.ErrorMessage = nil
		return &dto.SendNotificationResult{
			Notification: notif,
			Success:      true,
			Message:      fmt.Sprintf("delivered to %d of %d active subscriptions", sentCount, len(subs)),
		}
	}

	if lastErrMsg == "" {
		lastErrMsg = "no push subscriptions succeeded"
	}
	_ = s.historyRepo.UpdateDeliveryStatus(ctx, notif.ID, model.DeliveryStatusFailed, &lastErrMsg, hadRetryableErr)
	notif.Status = model.DeliveryStatusFailed
	notif.ErrorMessage = &lastErrMsg
	if hadRetryableErr {
		notif.RetryCount++
	}
	return &dto.SendNotificationResult{
		Notification: notif,
		Success:      false,
		Message:      lastErrMsg,
	}
}

func (s *notificationService) GetNotifications(ctx context.Context, filter model.NotificationFilter) (*dto.ListNotificationsResult, error) {
	filter.UserID = strings.TrimSpace(filter.UserID)
	if _, err := uuid.Parse(filter.UserID); err != nil {
		return nil, appErrors.BadRequest("valid user_id is required")
	}

	if filter.Status != "" {
		normalized := strings.ToUpper(strings.TrimSpace(filter.Status))
		switch model.DeliveryStatus(normalized) {
		case model.DeliveryStatusPending, model.DeliveryStatusSent, model.DeliveryStatusFailed:
			filter.Status = normalized
		default:
			return nil, appErrors.BadRequest("invalid status filter: must be PENDING, SENT, or FAILED")
		}
	}

	if filter.Channel != "" {
		filter.Channel = strings.ToUpper(strings.TrimSpace(filter.Channel))
		switch filter.Channel {
		case model.ChannelEmail, model.ChannelSMS, model.ChannelInApp, model.ChannelPush:
		default:
			return nil, appErrors.BadRequest("invalid channel filter: must be EMAIL, SMS, IN_APP, or PUSH")
		}
	}

	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	items, total, unread, err := s.historyRepo.ListNotifications(ctx, filter)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to retrieve notification history")
	}

	return &dto.ListNotificationsResult{
		Notifications: items,
		Total:         total,
		UnreadCount:   unread,
	}, nil
}

func (s *notificationService) MarkAsRead(ctx context.Context, notificationID, userID string, isRead bool) (*model.Notification, error) {
	nID := strings.TrimSpace(notificationID)
	if _, err := uuid.Parse(nID); err != nil {
		return nil, appErrors.BadRequest("valid notification_id is required")
	}
	uID := strings.TrimSpace(userID)
	if _, err := uuid.Parse(uID); err != nil {
		return nil, appErrors.BadRequest("valid user_id is required")
	}

	existing, err := s.historyRepo.GetNotificationByID(ctx, nID)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to look up notification")
	}
	if existing == nil {
		return nil, appErrors.NotFound("notification not found")
	}
	if existing.UserID != uID {
		return nil, appErrors.Forbidden("not authorized to modify this notification")
	}

	updated, err := s.historyRepo.MarkAsRead(ctx, nID, uID, isRead)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to update notification read status")
	}
	if updated == nil {
		return nil, appErrors.NotFound("notification not found")
	}
	return updated, nil
}

func (s *notificationService) MarkAllAsRead(ctx context.Context, userID string) (int, error) {
	uID := strings.TrimSpace(userID)
	if _, err := uuid.Parse(uID); err != nil {
		return 0, appErrors.BadRequest("valid user_id is required")
	}

	count, err := s.historyRepo.MarkAllAsRead(ctx, uID)
	if err != nil {
		return 0, appErrors.Internal(err, "failed to mark all notifications as read")
	}
	return count, nil
}

func (s *notificationService) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	uID := strings.TrimSpace(userID)
	if _, err := uuid.Parse(uID); err != nil {
		return 0, appErrors.BadRequest("valid user_id is required")
	}

	count, err := s.historyRepo.GetUnreadCount(ctx, uID)
	if err != nil {
		return 0, appErrors.Internal(err, "failed to get unread notification count")
	}
	return count, nil
}
