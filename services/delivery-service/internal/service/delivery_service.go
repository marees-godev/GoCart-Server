package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/repository"
)

type DeliveryService interface {
	HandleOrderConfirmed(ctx context.Context, payload *dto.OrderConfirmedPayload) (*model.Delivery, error)
	GetDeliveryByOrderID(ctx context.Context, orderID string) (*model.Delivery, error)
	GetDeliveryByTrackingNumber(ctx context.Context, trackingNumber string) (*model.Delivery, error)
	GetDeliveryByID(ctx context.Context, id string) (*model.Delivery, error)
	ListDeliveries(ctx context.Context, limit, offset int, status string) ([]*model.Delivery, int, error)
	UpdateStatus(ctx context.Context, id string, targetStatus model.DeliveryStatus) (*model.Delivery, error)
	GetTempOrderByID(ctx context.Context, orderID string) (*model.TempOrder, error)
	UpsertTempOrder(ctx context.Context, order *model.TempOrder) error
}

type deliveryService struct {
	repo   repository.DeliveryRepository
	logger *slog.Logger
}

func NewDeliveryService(repo repository.DeliveryRepository, log ...*slog.Logger) DeliveryService {
	var l *slog.Logger
	if len(log) > 0 && log[0] != nil {
		l = log[0]
	} else {
		l = slog.Default()
	}
	return &deliveryService{
		repo:   repo,
		logger: l,
	}
}

func (s *deliveryService) HandleOrderConfirmed(ctx context.Context, payload *dto.OrderConfirmedPayload) (*model.Delivery, error) {
	if payload == nil || strings.TrimSpace(payload.OrderID) == "" {
		s.logger.Warn("HandleOrderConfirmed failed: order_id is required")
		return nil, appErrors.BadRequest("order_id is required")
	}

	orderID := strings.TrimSpace(payload.OrderID)

	// Idempotency check: return existing delivery if already created
	existing, err := s.repo.GetByOrderID(ctx, orderID)
	if err == nil && existing != nil {
		s.logger.Info("Delivery already exists for order (idempotent)", slog.String("order_id", orderID), slog.String("delivery_id", existing.ID))
		return existing, nil
	}

	var addr model.ShippingAddress
	if payload.ShippingAddress != nil {
		addr = *payload.ShippingAddress
	}

	if strings.TrimSpace(addr.ContactNumber) == "" {
		s.logger.Warn("HandleOrderConfirmed failed: contact_number is required in shipping address", slog.String("order_id", orderID))
		return nil, appErrors.BadRequest("contact_number is required in shipping address")
	}

	partner := strings.TrimSpace(payload.DeliveryPartner)
	if partner == "" {
		partner = "STANDARD"
	}

	trackingNumber, err := s.generateTrackingNumber(partner)
	if err != nil {
		s.logger.Error("Failed to generate tracking number", slog.Any("error", err))
		return nil, appErrors.Internal(err, "failed to generate tracking number")
	}

	now := time.Now().UTC()
	delivery := &model.Delivery{
		ID:              uuid.NewString(),
		OrderID:         orderID,
		DeliveryPartner: partner,
		TrackingNumber:  trackingNumber,
		ShippingAddress: addr,
		Status:          model.StatusCreated,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	s.logger.Info("Creating delivery record", slog.String("order_id", orderID), slog.String("partner", partner), slog.String("tracking_number", trackingNumber))
	if err := s.repo.Create(ctx, delivery); err != nil {
		s.logger.Error("Failed to persist delivery record", slog.String("order_id", orderID), slog.Any("error", err))
		return nil, err
	}

	persisted, err := s.repo.GetByOrderID(ctx, orderID)
	if err == nil && persisted != nil {
		return persisted, nil
	}

	return delivery, nil
}

func (s *deliveryService) GetDeliveryByOrderID(ctx context.Context, orderID string) (*model.Delivery, error) {
	cleanID := strings.TrimSpace(orderID)
	if cleanID == "" {
		s.logger.Warn("GetDeliveryByOrderID failed: order_id is required")
		return nil, appErrors.BadRequest("order_id is required")
	}
	s.logger.Info("Retrieving delivery by order ID", slog.String("order_id", cleanID))
	return s.repo.GetByOrderID(ctx, cleanID)
}

func (s *deliveryService) GetDeliveryByTrackingNumber(ctx context.Context, trackingNumber string) (*model.Delivery, error) {
	cleanNum := strings.TrimSpace(trackingNumber)
	if cleanNum == "" {
		s.logger.Warn("GetDeliveryByTrackingNumber failed: tracking_number is required")
		return nil, appErrors.BadRequest("tracking_number is required")
	}
	s.logger.Info("Retrieving delivery by tracking number", slog.String("tracking_number", cleanNum))
	return s.repo.GetByTrackingNumber(ctx, cleanNum)
}

func (s *deliveryService) GetDeliveryByID(ctx context.Context, id string) (*model.Delivery, error) {
	cleanID := strings.TrimSpace(id)
	if cleanID == "" {
		s.logger.Warn("GetDeliveryByID failed: id is required")
		return nil, appErrors.BadRequest("id is required")
	}
	s.logger.Info("Retrieving delivery by ID", slog.String("delivery_id", cleanID))
	delivery, err := s.repo.GetByID(ctx, cleanID)
	if err != nil && appErrors.IsNotFound(err) {
		orderDelivery, orderErr := s.repo.GetByOrderID(ctx, cleanID)
		if orderErr == nil && orderDelivery != nil {
			return orderDelivery, nil
		}
	}
	return delivery, err
}

func (s *deliveryService) ListDeliveries(ctx context.Context, limit, offset int, status string) ([]*model.Delivery, int, error) {
	s.logger.Info("Listing deliveries", slog.Int("limit", limit), slog.Int("offset", offset), slog.String("status", status))
	return s.repo.List(ctx, limit, offset, status)
}

func (s *deliveryService) UpdateStatus(ctx context.Context, id string, targetStatus model.DeliveryStatus) (*model.Delivery, error) {
	cleanID := strings.TrimSpace(id)
	if cleanID == "" {
		s.logger.Warn("UpdateStatus failed: id is required")
		return nil, appErrors.BadRequest("id is required")
	}

	cleanTargetStatus := model.DeliveryStatus(strings.ToUpper(strings.TrimSpace(string(targetStatus))))
	if cleanTargetStatus == "" {
		s.logger.Warn("UpdateStatus failed: target status is required")
		return nil, appErrors.BadRequest("status is required")
	}

	delivery, err := s.repo.GetByID(ctx, cleanID)
	if err != nil && appErrors.IsNotFound(err) {
		delivery, err = s.repo.GetByOrderID(ctx, cleanID)
	}
	if err != nil {
		s.logger.Error("UpdateStatus failed: delivery not found", slog.String("delivery_id", cleanID), slog.Any("error", err))
		return nil, err
	}

	if delivery.Status == cleanTargetStatus {
		s.logger.Info("Delivery is already in target status (idempotent)", slog.String("delivery_id", delivery.ID), slog.String("status", string(cleanTargetStatus)))
		return delivery, nil
	}

	if err := model.TransitionStatus(delivery.Status, cleanTargetStatus); err != nil {
		s.logger.Warn("Invalid delivery state transition", slog.String("delivery_id", delivery.ID), slog.String("current_status", string(delivery.Status)), slog.String("target_status", string(cleanTargetStatus)), slog.Any("error", err))
		var transErr *model.InvalidStateTransitionError
		if errors.As(err, &transErr) {
			return nil, transErr.ToAppError()
		}
		return nil, appErrors.New(appErrors.CodeUnprocessableEntity, err.Error(), http.StatusUnprocessableEntity)
	}

	s.logger.Info("Transitioning delivery status", slog.String("delivery_id", delivery.ID), slog.String("current_status", string(delivery.Status)), slog.String("target_status", string(cleanTargetStatus)))
	return s.repo.UpdateStatus(ctx, delivery.ID, cleanTargetStatus)
}

func (s *deliveryService) GetTempOrderByID(ctx context.Context, orderID string) (*model.TempOrder, error) {
	cleanID := strings.TrimSpace(orderID)
	if cleanID == "" {
		s.logger.Warn("GetTempOrderByID failed: order_id is required")
		return nil, appErrors.BadRequest("order_id is required")
	}
	s.logger.Info("Retrieving temporary order by ID", slog.String("order_id", cleanID))
	return s.repo.GetTempOrderByID(ctx, cleanID)
}

func (s *deliveryService) UpsertTempOrder(ctx context.Context, order *model.TempOrder) error {
	if order == nil || strings.TrimSpace(order.ID) == "" {
		s.logger.Warn("UpsertTempOrder failed: valid order is required")
		return appErrors.BadRequest("order is required")
	}
	s.logger.Info("Upserting temporary order", slog.String("order_id", order.ID))
	return s.repo.UpsertTempOrder(ctx, order)
}

func (s *deliveryService) generateTrackingNumber(partner string) (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	prefix := "TRK"
	if len(partner) >= 3 {
		prefix = strings.ToUpper(partner[:3])
	}
	return fmt.Sprintf("GC-%s-%s", prefix, strings.ToUpper(hex.EncodeToString(b))), nil
}
