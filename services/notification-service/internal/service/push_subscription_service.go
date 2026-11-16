package service

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/push"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
)

type PushSubscriptionService interface {
	Register(ctx context.Context, input dto.RegisterPushSubscriptionInput) (*model.PushSubscription, error)
	GetByUserID(ctx context.Context, userID string, activeOnly bool) ([]*model.PushSubscription, error)
	Update(ctx context.Context, input dto.UpdatePushSubscriptionInput) (*model.PushSubscription, error)
	Deactivate(ctx context.Context, input dto.DeactivatePushSubscriptionInput) (bool, error)
	GetVAPIDPublicKey(ctx context.Context) (publicKey string, subject string, err error)
}

type pushSubscriptionService struct {
	subRepo repository.PushSubscriptionRepository
	vapid   config.VAPIDConfig
	logger  *slog.Logger
}

func NewPushSubscriptionService(
	subRepo repository.PushSubscriptionRepository,
	vapid config.VAPIDConfig,
	logger *slog.Logger,
) PushSubscriptionService {
	if logger == nil {
		logger = slog.Default()
	}
	return &pushSubscriptionService{
		subRepo: subRepo,
		vapid:   vapid,
		logger:  logger,
	}
}

func (s *pushSubscriptionService) Register(ctx context.Context, input dto.RegisterPushSubscriptionInput) (*model.PushSubscription, error) {
	userID := strings.TrimSpace(input.UserID)
	if _, err := uuid.Parse(userID); err != nil {
		return nil, appErrors.BadRequest("valid user_id is required")
	}

	endpoint := strings.TrimSpace(input.Endpoint)
	if err := validateEndpointURL(endpoint); err != nil {
		return nil, appErrors.BadRequest(err.Error())
	}

	p256dh := strings.TrimSpace(input.P256DH)
	authSecret := strings.TrimSpace(input.Auth)
	if err := push.ValidateSubscriptionKeys(p256dh, authSecret); err != nil {
		return nil, appErrors.BadRequest(err.Error())
	}

	sub := &model.PushSubscription{
		UserID:     userID,
		Endpoint:   endpoint,
		P256DH:     p256dh,
		Auth:       authSecret,
		UserAgent:  truncateString(strings.TrimSpace(input.UserAgent), 512),
		DeviceName: truncateString(strings.TrimSpace(input.DeviceName), 255),
		IsActive:   true,
	}

	saved, err := s.subRepo.UpsertSubscription(ctx, sub)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to register push subscription")
	}

	s.logger.InfoContext(ctx, "Push subscription registered",
		"subscription_id", saved.ID,
		"user_id", saved.UserID,
		"device_name", saved.DeviceName,
		"is_active", saved.IsActive,
	)

	return saved, nil
}

func (s *pushSubscriptionService) GetByUserID(ctx context.Context, userID string, activeOnly bool) ([]*model.PushSubscription, error) {
	trimmed := strings.TrimSpace(userID)
	if _, err := uuid.Parse(trimmed); err != nil {
		return nil, appErrors.BadRequest("valid user_id is required")
	}

	subs, err := s.subRepo.ListSubscriptionsByUserID(ctx, trimmed, activeOnly)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to list push subscriptions")
	}
	return subs, nil
}

func (s *pushSubscriptionService) Update(ctx context.Context, input dto.UpdatePushSubscriptionInput) (*model.PushSubscription, error) {
	userID := strings.TrimSpace(input.UserID)
	if _, err := uuid.Parse(userID); err != nil {
		return nil, appErrors.BadRequest("valid user_id is required")
	}

	id := strings.TrimSpace(input.ID)
	endpoint := strings.TrimSpace(input.Endpoint)
	if id == "" && endpoint == "" {
		return nil, appErrors.BadRequest("subscription id or endpoint is required")
	}

	var (
		existing *model.PushSubscription
		err      error
	)
	if id != "" {
		if _, parseErr := uuid.Parse(id); parseErr != nil {
			return nil, appErrors.BadRequest("invalid subscription id")
		}
		existing, err = s.subRepo.GetSubscriptionByID(ctx, id)
	} else {
		if urlErr := validateEndpointURL(endpoint); urlErr != nil {
			return nil, appErrors.BadRequest(urlErr.Error())
		}
		existing, err = s.subRepo.GetSubscriptionByEndpoint(ctx, endpoint)
	}
	if err != nil {
		return nil, appErrors.Internal(err, "failed to look up push subscription")
	}
	if existing == nil {
		return nil, appErrors.NotFound("push subscription not found")
	}
	if existing.UserID != userID {
		return nil, appErrors.Forbidden("not authorized to update this push subscription")
	}

	if endpoint != "" {
		if err := validateEndpointURL(endpoint); err != nil {
			return nil, appErrors.BadRequest(err.Error())
		}
	}

	newP256DH := strings.TrimSpace(input.P256DH)
	newAuth := strings.TrimSpace(input.Auth)
	if newP256DH != "" || newAuth != "" {
		checkP256DH := existing.P256DH
		if newP256DH != "" {
			checkP256DH = newP256DH
		}
		checkAuth := existing.Auth
		if newAuth != "" {
			checkAuth = newAuth
		}
		if err := push.ValidateSubscriptionKeys(checkP256DH, checkAuth); err != nil {
			return nil, appErrors.BadRequest(err.Error())
		}
	}

	isActive := existing.IsActive
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	patch := &model.PushSubscription{
		ID:         existing.ID,
		UserID:     userID,
		Endpoint:   endpoint,
		P256DH:     newP256DH,
		Auth:       newAuth,
		UserAgent:  truncateString(strings.TrimSpace(input.UserAgent), 512),
		DeviceName: truncateString(strings.TrimSpace(input.DeviceName), 255),
		IsActive:   isActive,
	}

	updated, err := s.subRepo.UpdateSubscription(ctx, patch)
	if err != nil {
		if strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "already exists") {
			return nil, appErrors.Conflict("endpoint already registered")
		}
		return nil, appErrors.Internal(err, "failed to update push subscription")
	}
	if updated == nil {
		return nil, appErrors.NotFound("push subscription not found")
	}

	s.logger.InfoContext(ctx, "Push subscription updated",
		"subscription_id", updated.ID,
		"user_id", updated.UserID,
		"is_active", updated.IsActive,
	)

	return updated, nil
}

func (s *pushSubscriptionService) Deactivate(ctx context.Context, input dto.DeactivatePushSubscriptionInput) (bool, error) {
	userID := strings.TrimSpace(input.UserID)
	if _, err := uuid.Parse(userID); err != nil {
		return false, appErrors.BadRequest("valid user_id is required")
	}

	id := strings.TrimSpace(input.ID)
	endpoint := strings.TrimSpace(input.Endpoint)
	if id == "" && endpoint == "" {
		return false, appErrors.BadRequest("subscription id or endpoint is required")
	}

	var (
		existing *model.PushSubscription
		err      error
	)
	if id != "" {
		if _, parseErr := uuid.Parse(id); parseErr != nil {
			return false, appErrors.BadRequest("invalid subscription id")
		}
		existing, err = s.subRepo.GetSubscriptionByID(ctx, id)
	} else {
		existing, err = s.subRepo.GetSubscriptionByEndpoint(ctx, endpoint)
	}
	if err != nil {
		return false, appErrors.Internal(err, "failed to look up push subscription")
	}
	if existing == nil {
		return false, appErrors.NotFound("push subscription not found")
	}
	if existing.UserID != userID {
		return false, appErrors.Forbidden("not authorized to deactivate this push subscription")
	}

	deactivated, err := s.subRepo.DeactivateSubscription(ctx, existing.ID, userID, "")
	if err != nil {
		return false, appErrors.Internal(err, "failed to deactivate push subscription")
	}

	s.logger.InfoContext(ctx, "Push subscription deactivated",
		"subscription_id", existing.ID,
		"user_id", userID,
		"deactivated", deactivated,
	)

	return deactivated, nil
}

func (s *pushSubscriptionService) GetVAPIDPublicKey(ctx context.Context) (string, string, error) {
	pubKey := strings.TrimSpace(s.vapid.PublicKey)
	if pubKey == "" {
		return "", "", appErrors.ServiceUnavailable("web push VAPID public key is not configured")
	}
	if err := push.ValidateVAPIDPublicKey(pubKey); err != nil {
		return "", "", appErrors.Internal(err, "configured VAPID public key is invalid")
	}
	subject := strings.TrimSpace(s.vapid.Subject)
	if subject == "" {
		subject = "mailto:notifications@gocart.local"
	}
	return pubKey, subject, nil
}

func validateEndpointURL(rawURL string) error {
	if rawURL == "" {
		return appErrors.BadRequest("endpoint is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return appErrors.BadRequest("endpoint must be a valid http or https URL with host")
	}
	return nil
}

func truncateString(s string, maxRunes int) string {
	if maxRunes <= 0 || len(s) == 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}
