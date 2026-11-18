package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/kafka"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/repository"
)

type PersonalizationService interface {
	TrackActivity(ctx context.Context, input *dto.TrackActivityInput) error
	ProcessEvent(ctx context.Context, envelope *events.EventEnvelope) error
	GetCustomerActivities(ctx context.Context, userID, sessionID string, limit, offset int) ([]*model.CustomerActivity, int, error)
	GetRecentlyViewedProducts(ctx context.Context, userID, sessionID string, limit, offset int) ([]*dto.RecentlyViewedProduct, int, error)
	GetCategoryPreference(ctx context.Context, userID, sessionID string, limit, offset int) ([]*dto.CustomerPreference, error)
}

type personalizationService struct {
	repo     repository.ActivityRepository
	producer *kafka.Producer
	log      *slog.Logger
}

func NewPersonalizationService(repo repository.ActivityRepository, producer *kafka.Producer, log *slog.Logger) PersonalizationService {
	return &personalizationService{
		repo:     repo,
		producer: producer,
		log:      log,
	}
}

func (s *personalizationService) TrackActivity(ctx context.Context, input *dto.TrackActivityInput) error {
	if input == nil {
		return fmt.Errorf("input cannot be nil")
	}

	if input.UserID == "" && input.SessionID == "" {
		return fmt.Errorf("either user_id or session_id must be provided")
	}

	switch input.EventType {
	case events.EventTypeCategoryViewed:
		if input.CategoryID == "" {
			return fmt.Errorf("category_id is required for CategoryViewed")
		}
	case events.EventTypeProductViewed, events.EventTypeProductAddedToCart,
		events.EventTypeProductRemovedFromCart, events.EventTypeProductWishlisted,
		events.EventTypeProductPurchased:
		if input.ProductID == "" {
			return fmt.Errorf("product_id is required for %s", input.EventType)
		}
	case events.EventTypeProductSearched:
		// ProductSearched captures category_id or product_id of the search
	default:
		return fmt.Errorf("unsupported event type: %s", input.EventType)
	}

	payload := events.CustomerActivityEvent{
		UserID:     input.UserID,
		SessionID:  input.SessionID,
		EventType:  input.EventType,
		ProductID:  input.ProductID,
		CategoryID: input.CategoryID,
		Timestamp:  time.Now().UTC(),
	}

	envelope, err := events.NewEventEnvelope(input.EventType, "personalization-service", payload)
	if err != nil {
		return fmt.Errorf("failed to create event envelope: %w", err)
	}

	partitionKey := input.UserID
	if partitionKey == "" {
		partitionKey = input.SessionID
	}

	if s.producer != nil {
		if err := s.producer.Publish(ctx, events.TopicCustomerActivity, partitionKey, envelope); err != nil {
			s.log.Error("Failed to publish event to kafka", "error", err)
			return err
		}
	}

	return nil
}

// ProcessEvent is called by the background Kafka consumer to save the event to DB
func (s *personalizationService) ProcessEvent(ctx context.Context, envelope *events.EventEnvelope) error {
	if envelope == nil {
		return nil
	}

	processed, err := s.repo.IsEventProcessed(ctx, envelope.EventID)
	if err != nil {
		s.log.Error("Database error during idempotency check", "error", err, "event_id", envelope.EventID)
		return err
	}
	if processed {
		s.log.Info("Duplicate event skipped", "event_id", envelope.EventID)
		return nil
	}

	// Process all interaction events
	switch envelope.EventType {
	case events.EventTypeProductViewed, events.EventTypeProductSearched,
		events.EventTypeCategoryViewed, events.EventTypeProductAddedToCart,
		events.EventTypeProductRemovedFromCart, events.EventTypeProductWishlisted,
		events.EventTypeProductPurchased:
		var payload events.CustomerActivityEvent
		if err := json.Unmarshal(envelope.Data, &payload); err != nil {
			s.log.Error("Failed to unmarshal CustomerActivityEvent payload", "error", err)
			return nil // Don't retry malformed JSON
		}

		var userPtr *string
		if payload.UserID != "" {
			userPtr = &payload.UserID
		}
		var prodPtr *string
		if payload.ProductID != "" {
			prodPtr = &payload.ProductID
		}
		var catPtr *string
		if payload.CategoryID != "" {
			catPtr = &payload.CategoryID
		}
		var sessPtr *string
		if payload.SessionID != "" {
			sessPtr = &payload.SessionID
		}

		activity := &model.CustomerActivity{
			UserID:     userPtr,
			SessionID:  sessPtr,
			EventType:  mapEventToDBType(envelope.EventType),
			ProductID:  prodPtr,
			CategoryID: catPtr,
		}

		if err := s.repo.SaveActivity(ctx, envelope.EventID, activity); err != nil {
			s.log.Error("Failed to save customer activity", "error", err)
			return err
		}
		s.log.Info("Successfully tracked customer activity", "event_type", envelope.EventType, "session_id", payload.SessionID, "event_id", envelope.EventID)
	}
	return nil
}

func (s *personalizationService) GetCustomerActivities(ctx context.Context, userID, sessionID string, limit, offset int) ([]*model.CustomerActivity, int, error) {
	if userID == "" && sessionID == "" {
		return nil, 0, fmt.Errorf("either user_id or session_id must be provided")
	}

	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.GetCustomerActivities(ctx, userID, sessionID, limit, offset)
}

func (s *personalizationService) GetRecentlyViewedProducts(ctx context.Context, userID, sessionID string, limit, offset int) ([]*dto.RecentlyViewedProduct, int, error) {
	if userID == "" && sessionID == "" {
		return nil, 0, fmt.Errorf("either user_id or session_id must be provided")
	}

	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	return s.repo.GetRecentlyViewedProducts(ctx, userID, sessionID, limit, offset)
}

func (s *personalizationService) GetCategoryPreference(ctx context.Context, userID, sessionID string, limit, offset int) ([]*dto.CustomerPreference, error) {
	if userID == "" && sessionID == "" {
		return nil, fmt.Errorf("either user_id or session_id must be provided")
	}

	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	return s.repo.GetCategoryPreference(ctx, userID, sessionID, limit, offset)
}

func mapEventToDBType(eventType string) string {
	switch eventType {
	case events.EventTypeProductViewed:
		return "PRODUCT_VIEWED"
	case events.EventTypeProductSearched:
		return "PRODUCT_SEARCHED"
	case events.EventTypeCategoryViewed:
		return "CATEGORY_VIEWED"
	case events.EventTypeProductAddedToCart:
		return "PRODUCT_ADDED_TO_CART"
	case events.EventTypeProductRemovedFromCart:
		return "PRODUCT_REMOVED_FROM_CART"
	case events.EventTypeProductWishlisted:
		return "PRODUCT_WISHLISTED"
	case events.EventTypeProductPurchased:
		return "PRODUCT_PURCHASED"
	default:
		return eventType
	}
}
