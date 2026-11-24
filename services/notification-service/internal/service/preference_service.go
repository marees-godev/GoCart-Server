package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
)

type PreferenceService interface {
	CheckPreferenceAndSelectChannel(ctx context.Context, userID, eventType, requestedChannelCode string) (*model.Channel, bool, error)
	GetUserPreferences(ctx context.Context, userID string) ([]*model.Preference, error)
	UpdateUserPreference(ctx context.Context, userID, channelCode, category string, isEnabled bool) (*model.Preference, error)
}

type preferenceService struct {
	channelRepo    repository.ChannelRepository
	preferenceRepo repository.PreferenceRepository
}

func NewPreferenceService(
	channelRepo repository.ChannelRepository,
	preferenceRepo repository.PreferenceRepository,
) PreferenceService {
	return &preferenceService{
		channelRepo:    channelRepo,
		preferenceRepo: preferenceRepo,
	}
}

const (
	CategoryOrders     = "ORDERS"
	CategoryDeliveries = "DELIVERIES"
	CategoryReturns    = "RETURNS"
	CategoryAccount    = "ACCOUNT"
	CategoryPayments   = "PAYMENTS"
)

func NormalizeEventType(eventType string) string {
	clean := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(eventType), ".", ""), "_", ""))
	switch clean {
	case "ordercreated", "orderplaced":
		return "OrderCreated"
	case "orderconfirmed":
		return "OrderConfirmed"
	case "ordershipped":
		return "OrderShipped"
	case "orderdelivered":
		return "OrderDelivered"
	case "ordercancelled", "ordercanceled":
		return "OrderCancelled"
	case "orderpaymentfailed":
		return "OrderPaymentFailed"
	case "paymentsuccessful", "paymentsucceeded":
		return "PaymentSuccessful"
	case "paymentfailed":
		return "PaymentFailed"
	case "deliverycreated":
		return "DeliveryCreated"
	case "deliverystatusupdated", "deliverystatuschanged":
		return "DeliveryStatusUpdated"
	case "returnapproved":
		return "ReturnApproved"
	case "returnrejected":
		return "ReturnRejected"
	case "refundcompleted":
		return "RefundCompleted"
	case "userregistered":
		return "UserRegistered"
	case "storeapproved":
		return "StoreApproved"
	case "storerejected":
		return "StoreRejected"
	case "merchantapproved":
		return "MerchantApproved"
	case "merchantactivated":
		return "MerchantActivated"
	case "merchantrejected":
		return "MerchantRejected"
	case "merchantsuspended":
		return "MerchantSuspended"
	default:
		return strings.TrimSpace(eventType)
	}
}

func EventToCategory(eventType string) string {
	switch NormalizeEventType(eventType) {
	case "OrderCreated", "OrderConfirmed", "OrderShipped", "OrderDelivered", "OrderCancelled":
		return CategoryOrders
	case "DeliveryCreated", "DeliveryStatusUpdated", "DeliveryStatusChanged":
		return CategoryDeliveries
	case "ReturnApproved", "ReturnRejected", "RefundCompleted":
		return CategoryReturns
	case "PaymentFailed", "OrderPaymentFailed", "PaymentSuccessful":
		return CategoryPayments
	default:
		return CategoryAccount
	}
}

func isCriticalNotification(eventType string) bool {
	return EventToCategory(eventType) == CategoryPayments
}

func (s *preferenceService) CheckPreferenceAndSelectChannel(
	ctx context.Context,
	userID string,
	eventType string,
	requestedChannelCode string,
) (*model.Channel, bool, error) {
	if requestedChannelCode == "" {
		requestedChannelCode = "EMAIL"
	}

	channel, err := s.channelRepo.GetByCode(ctx, requestedChannelCode)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get channel %s: %w", requestedChannelCode, err)
	}
	if channel == nil {
		channel, err = s.channelRepo.GetByCode(ctx, "EMAIL")
		if err != nil {
			return nil, false, fmt.Errorf("failed to get default email channel: %w", err)
		}
		if channel == nil {
			return nil, false, fmt.Errorf("no active notification channel available")
		}
	}

	if userID == "" {
		return channel, true, nil
	}

	if isCriticalNotification(eventType) {
		return channel, true, nil
	}

	category := EventToCategory(eventType)
	pref, err := s.preferenceRepo.GetPreference(ctx, userID, channel.ID, category)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check preference: %w", err)
	}
	if pref != nil && !pref.IsEnabled {
		return channel, false, nil
	}

	return channel, true, nil
}

func (s *preferenceService) GetUserPreferences(ctx context.Context, userID string) ([]*model.Preference, error) {
	cleanUID := strings.TrimSpace(userID)
	if _, err := uuid.Parse(cleanUID); err != nil {
		return nil, appErrors.BadRequest("valid user_id is required")
	}
	return s.preferenceRepo.GetPreferencesByUserID(ctx, userID)
}

func (s *preferenceService) UpdateUserPreference(ctx context.Context, userID string, channelCode string, category string, isEnabled bool) (*model.Preference, error) {
	cleanUID := strings.TrimSpace(userID)
	if _, err := uuid.Parse(cleanUID); err != nil {
		return nil, appErrors.BadRequest("valid user_id is required")
	}

	normalizedChannel := strings.ToUpper(strings.TrimSpace(channelCode))
	if normalizedChannel != model.ChannelEmail && normalizedChannel != model.ChannelPush {
		return nil, appErrors.BadRequest(fmt.Sprintf("unsupported channel: %s, only EMAIL and PUSH are supported", channelCode))
	}

	normalizedCategory := strings.ToUpper(strings.TrimSpace(category))
	switch normalizedCategory {
	case CategoryOrders, CategoryDeliveries, CategoryReturns, CategoryAccount:
	case CategoryPayments:
		return nil, appErrors.BadRequest("critical transactional notifications such as payment failure cannot be disabled")
	default:
		return nil, appErrors.BadRequest(fmt.Sprintf("unsupported category: %s, supported categories are ORDERS, DELIVERIES, RETURNS, ACCOUNT", category))
	}

	channel, err := s.channelRepo.GetByCode(ctx, normalizedChannel)
	if err != nil || channel == nil {
		return nil, appErrors.BadRequest(fmt.Sprintf("channel not found: %s", normalizedChannel))
	}

	pref := &model.Preference{
		ID:        uuid.NewString(),
		UserID:    cleanUID,
		ChannelID: channel.ID,
		Category:  normalizedCategory,
		IsEnabled: isEnabled,
	}

	if err := s.preferenceRepo.UpsertPreference(ctx, pref); err != nil {
		return nil, fmt.Errorf("failed to save preference: %w", err)
	}

	return pref, nil
}