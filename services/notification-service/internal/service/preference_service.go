package service

import (
	"context"
	"fmt"

	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
)

type PreferenceService interface {
	CheckPreferenceAndSelectChannel(ctx context.Context, userID, eventType, requestedChannelCode string) (*model.Channel, bool, error)
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

	pref, err := s.preferenceRepo.GetPreference(ctx, userID, channel.ID, eventType)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check preference: %w", err)
	}

	if pref != nil && !pref.IsEnabled {
		return channel, false, nil
	}

	return channel, true, nil
}

