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
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/client"
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
	ProcessStatusUpdate(ctx context.Context, req *dto.DeliveryStatusUpdateRequest) (*model.Delivery, error)

	// Estimated delivery using order_service and transit rules
	EstimateDelivery(ctx context.Context, orderID string, partnerID ...string) (*dto.EstimatedDeliveryResponse, error)

	// Partner Management
	CreateOrUpdatePartnerProfile(ctx context.Context, profile *model.DeliveryPartnerProfile) error
	GetPartnerProfile(ctx context.Context, partnerID string) (*model.DeliveryPartnerProfile, error)
	UpdatePartnerProfile(ctx context.Context, partnerID string, companyName, email, contactNumber *string) (*model.DeliveryPartnerProfile, error)
}

type deliveryService struct {
	repo            repository.DeliveryRepository
	orderClient     client.OrderClient
	inventoryClient client.InventoryClient
	logger          *slog.Logger
}

func NewDeliveryService(repo repository.DeliveryRepository, args ...any) DeliveryService {
	s := &deliveryService{
		repo:   repo,
		logger: slog.Default(),
	}
	for _, arg := range args {
		switch v := arg.(type) {
		case client.OrderClient:
			s.orderClient = v
		case client.InventoryClient:
			s.inventoryClient = v
		case *slog.Logger:
			if v != nil {
				s.logger = v
			}
		}
	}
	return s
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

	// Retrieve order from order_service if orderClient is configured and address info is needed or for authoritative enrichment
	if s.orderClient != nil {
		if ord, oErr := s.orderClient.GetOrder(ctx, orderID); oErr == nil && ord != nil {
			if strings.TrimSpace(addr.City) == "" && strings.TrimSpace(ord.ShippingAddress.City) != "" {
				addr.City = ord.ShippingAddress.City
			}
			if strings.TrimSpace(addr.State) == "" && strings.TrimSpace(ord.ShippingAddress.State) != "" {
				addr.State = ord.ShippingAddress.State
			}
			if strings.TrimSpace(addr.Street) == "" && strings.TrimSpace(ord.ShippingAddress.Street) != "" {
				addr.Street = ord.ShippingAddress.Street
			}
			if strings.TrimSpace(addr.PostalCode) == "" && strings.TrimSpace(ord.ShippingAddress.PostalCode) != "" {
				addr.PostalCode = ord.ShippingAddress.PostalCode
			}
			if strings.TrimSpace(addr.Country) == "" && strings.TrimSpace(ord.ShippingAddress.Country) != "" {
				addr.Country = ord.ShippingAddress.Country
			}
			if strings.TrimSpace(addr.ContactNumber) == "" && strings.TrimSpace(ord.ShippingAddress.ContactNumber) != "" {
				addr.ContactNumber = ord.ShippingAddress.ContactNumber
			}
		}
	}

	if strings.TrimSpace(addr.ContactNumber) == "" {
		s.logger.Warn("HandleOrderConfirmed failed: contact_number is required in shipping address", slog.String("order_id", orderID))
		return nil, appErrors.BadRequest("contact_number is required in shipping address")
	}

	model.NormalizeAddressFromTokens(&addr)

	rawPartner := strings.TrimSpace(payload.DeliveryPartner)
	var partnerUUID uuid.UUID
	var partnerName string
	if rawPartner != "" {
		if pUUID, pErr := uuid.Parse(rawPartner); pErr == nil {
			partnerUUID = pUUID
			if prof, profErr := s.repo.GetPartnerProfile(ctx, partnerUUID); profErr == nil && prof != nil && strings.TrimSpace(prof.CompanyName) != "" {
				partnerName = strings.TrimSpace(prof.CompanyName)
			}
		} else {
			partnerName = rawPartner
			if prof, profErr := s.repo.GetPartnerProfileByCompanyName(ctx, rawPartner); profErr == nil && prof != nil {
				partnerUUID = prof.PartnerID
			}
		}
	}
	if partnerName == "" {
		if rawPartner != "" {
			partnerName = rawPartner
		} else {
			partnerName = "STANDARD"
		}
	}

	now := time.Now().UTC()

	// Transit SLA Calculation:
	// Use warehouse location from delivery partner check availability and set estimated delivery date
	var originWarehouse *model.Warehouse
	var originCity, originState string

	var partnerIDStr string
	if partnerUUID != uuid.Nil {
		partnerIDStr = partnerUUID.String()
	}
	warehouses := s.fetchPartnerWarehouses(ctx, partnerIDStr)
	if len(warehouses) > 0 {
		if addr.City == "" {
			model.InferCityStateFromWarehouses(&addr, warehouses)
		}
		originWarehouse = resolveProvisionalWarehouse(warehouses, addr.City, addr.State)
	}
	if originWarehouse == nil {
		if avail := s.fetchAvailableWarehouses(ctx); len(avail) > 0 {
			if addr.City == "" {
				model.InferCityStateFromWarehouses(&addr, avail)
			}
			originWarehouse = resolveProvisionalWarehouse(avail, addr.City, addr.State)
		}
	}
	if originWarehouse != nil {
		originCity = originWarehouse.City
		originState = originWarehouse.State
		if addr.City == "" && originCity != "" {
			addr.City = originCity
		}
	}

	transitResult := CalculateTransitDeliveryDate(originCity, originState, addr.City, addr.State, now)
	targetDate := transitResult.TargetDate

	trackingNumber, err := s.generateTrackingNumber(partnerName)
	if err != nil {
		s.logger.Error("Failed to generate tracking number", slog.String("order_id", orderID), slog.Any("error", err))
		return nil, err
	}

	delivery := &model.Delivery{
		ID:                uuid.NewString(),
		OrderID:           orderID,
		DeliveryPartner:   partnerName,
		TrackingNumber:    trackingNumber,
		ShippingAddress:   addr,
		Status:            model.StatusCreated,
		EstimatedDelivery: &targetDate,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	s.logger.Info("Creating delivery record", slog.String("order_id", orderID), slog.String("partner", partnerName), slog.String("tracking_number", trackingNumber), slog.Time("estimated_delivery", targetDate))
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

func (s *deliveryService) EstimateDelivery(ctx context.Context, orderID string, partnerID ...string) (*dto.EstimatedDeliveryResponse, error) {
	cleanOrderID := strings.TrimSpace(orderID)
	if cleanOrderID == "" {
		return nil, appErrors.BadRequest("order_id is required")
	}

	var destCity, destState string
	var destAddr *model.ShippingAddress

	// 1. Fetch authoritative order from order_service
	if s.orderClient != nil {
		ord, err := s.orderClient.GetOrder(ctx, cleanOrderID)
		if err != nil {
			return nil, err
		}
		destCity = ord.ShippingAddress.City
		destState = ord.ShippingAddress.State
		destAddr = &ord.ShippingAddress
	} else {
		// Fallback to existing delivery if order_client is not configured
		del, err := s.repo.GetByOrderID(ctx, cleanOrderID)
		if err == nil && del != nil {
			destCity = del.ShippingAddress.City
			destState = del.ShippingAddress.State
			destAddr = &del.ShippingAddress
		} else {
			return nil, appErrors.NotFound("order not found in order service")
		}
	}

	partner := "STANDARD"
	if len(partnerID) > 0 && strings.TrimSpace(partnerID[0]) != "" {
		partner = strings.TrimSpace(partnerID[0])
	}

	now := time.Now().UTC()

	// 2. Check warehouse location from delivery partner and check availability
	var originWarehouse *model.Warehouse
	var originCity, originState string
	warehouses := s.fetchPartnerWarehouses(ctx, partner)
	if len(warehouses) > 0 {
		originWarehouse = resolveProvisionalWarehouse(warehouses, destCity, destState)
	}
	if originWarehouse == nil {
		if avail := s.fetchAvailableWarehouses(ctx); len(avail) > 0 {
			originWarehouse = resolveProvisionalWarehouse(avail, destCity, destState)
		}
	}
	if originWarehouse != nil {
		originCity = originWarehouse.City
		originState = originWarehouse.State
	}

	// 3. Calculate estimated transit delivery date:
	// - Same-city delivery: 1–2 days (target: 2 days)
	// - Within the same state: 2–4 days (target: 4 days)
	// - Inter-state delivery: 4–7 days (target: 7 days)
	transitResult := CalculateTransitDeliveryDate(originCity, originState, destCity, destState, now)

	return &dto.EstimatedDeliveryResponse{
		OrderID:           cleanOrderID,
		DeliveryPartner:   partner,
		DeliveryType:      transitResult.DeliveryType,
		TransitEstimate:   transitResult.EstimatedTransit,
		MinDays:           transitResult.MinDays,
		MaxDays:           transitResult.MaxDays,
		EstimatedDelivery: transitResult.TargetDate,
		OriginWarehouse:   originWarehouse,
		DestinationCity:   destCity,
		DestinationState:  destState,
		ShippingAddress:   destAddr,
	}, nil
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
	return s.ProcessStatusUpdate(ctx, &dto.DeliveryStatusUpdateRequest{
		DeliveryID: id,
		Status:     targetStatus,
	})
}

func (s *deliveryService) ProcessStatusUpdate(ctx context.Context, req *dto.DeliveryStatusUpdateRequest) (*model.Delivery, error) {
	if req == nil {
		s.logger.Warn("ProcessStatusUpdate failed: request is nil")
		return nil, appErrors.BadRequest("status update request is required")
	}

	cleanStatus := model.DeliveryStatus(strings.ToUpper(strings.TrimSpace(string(req.Status))))
	if !model.IsValidStatus(cleanStatus) {
		s.logger.Warn("ProcessStatusUpdate rejected: invalid or unknown status", slog.String("status", string(req.Status)))
		return nil, appErrors.BadRequest("invalid or unknown delivery status: " + string(req.Status))
	}

	identifier := strings.TrimSpace(req.TrackingNumber)
	if identifier == "" {
		identifier = strings.TrimSpace(req.DeliveryID)
	}
	if identifier == "" {
		identifier = strings.TrimSpace(req.OrderID)
	}
	if identifier == "" {
		s.logger.Warn("ProcessStatusUpdate failed: missing delivery identifier")
		return nil, appErrors.BadRequest("tracking_number, delivery_id, or order_id is required")
	}

	delivery, err := s.repo.GetByTrackingNumber(ctx, identifier)
	if err != nil && appErrors.IsNotFound(err) {
		delivery, err = s.repo.GetByID(ctx, identifier)
	}
	if err != nil && appErrors.IsNotFound(err) {
		delivery, err = s.repo.GetByOrderID(ctx, identifier)
	}
	if err != nil {
		s.logger.Error("ProcessStatusUpdate failed: delivery not found", slog.String("identifier", identifier), slog.Any("error", err))
		return nil, err
	}

	// Idempotency: Duplicate delivery events handled safely
	if delivery.Status == cleanStatus {
		s.logger.Info("Delivery is already in target status (idempotent)", slog.String("delivery_id", delivery.ID), slog.String("status", string(cleanStatus)))
		if req.EstimatedDelivery != nil || req.ActualDelivery != nil {
			return s.repo.UpdateStatusWithDetails(ctx, delivery.ID, cleanStatus, req.EstimatedDelivery, req.ActualDelivery, req.Location, req.Notes)
		}
		return delivery, nil
	}

	if err := model.TransitionStatus(delivery.Status, cleanStatus); err != nil {
		s.logger.Warn("Invalid delivery state transition", slog.String("delivery_id", delivery.ID), slog.String("current", string(delivery.Status)), slog.String("target", string(cleanStatus)), slog.Any("error", err))
		var transErr *model.InvalidStateTransitionError
		if errors.As(err, &transErr) {
			return nil, transErr.ToAppError()
		}
		return nil, appErrors.New(appErrors.CodeUnprocessableEntity, err.Error(), http.StatusUnprocessableEntity)
	}

	// If transitioning to DELIVERED and actual_delivery is not specified, record it
	actDelivery := req.ActualDelivery
	if cleanStatus == model.StatusDelivered && actDelivery == nil {
		now := time.Now().UTC()
		actDelivery = &now
	}

	s.logger.Info("Processing delivery status update", slog.String("delivery_id", delivery.ID), slog.String("current", string(delivery.Status)), slog.String("target", string(cleanStatus)))
	return s.repo.UpdateStatusWithDetails(ctx, delivery.ID, cleanStatus, req.EstimatedDelivery, actDelivery, req.Location, req.Notes)
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

func (s *deliveryService) fetchPartnerWarehouses(ctx context.Context, partnerID string) []*model.Warehouse {
	if s.inventoryClient != nil {
		whs, err := s.inventoryClient.GetPartnerWarehouses(ctx, partnerID)
		if err == nil && len(whs) > 0 {
			result := make([]*model.Warehouse, len(whs))
			for i, w := range whs {
				pUUID, _ := uuid.Parse(w.DeliveryPartnerID)
				wUUID, _ := uuid.Parse(w.ID)
				result[i] = &model.Warehouse{
					ID:                wUUID,
					DeliveryPartnerID: pUUID,
					Name:              w.Name,
					Code:              w.Code,
					City:              w.City,
					State:             w.State,
					IsActive:          w.IsActive,
				}
			}
			return result
		}
	}
	if s.repo != nil {
		if pUUID, err := uuid.Parse(partnerID); err == nil {
			if whs, err := s.repo.GetPartnerWarehouses(ctx, pUUID); err == nil && len(whs) > 0 {
				return whs
			}
		}
	}
	return nil
}

func (s *deliveryService) fetchAvailableWarehouses(ctx context.Context) []*model.Warehouse {
	if s.inventoryClient != nil {
		whs, err := s.inventoryClient.GetAvailableWarehouses(ctx)
		if err == nil && len(whs) > 0 {
			result := make([]*model.Warehouse, len(whs))
			for i, w := range whs {
				pUUID, _ := uuid.Parse(w.DeliveryPartnerID)
				wUUID, _ := uuid.Parse(w.ID)
				result[i] = &model.Warehouse{
					ID:                wUUID,
					DeliveryPartnerID: pUUID,
					Name:              w.Name,
					Code:              w.Code,
					City:              w.City,
					State:             w.State,
					IsActive:          w.IsActive,
				}
			}
			return result
		}
	}
	if s.repo != nil {
		if whs, err := s.repo.GetAvailableWarehouses(ctx); err == nil && len(whs) > 0 {
			return whs
		}
	}
	return nil
}

func (s *deliveryService) CreateOrUpdatePartnerProfile(ctx context.Context, profile *model.DeliveryPartnerProfile) error {
	if profile == nil {
		return appErrors.BadRequest("profile is required")
	}
	if profile.PartnerID == uuid.Nil {
		return appErrors.BadRequest("partner_id is required")
	}
	return s.repo.CreateOrUpdatePartnerProfile(ctx, profile)
}

func (s *deliveryService) GetPartnerProfile(ctx context.Context, partnerID string) (*model.DeliveryPartnerProfile, error) {
	targetPartner := strings.TrimSpace(partnerID)
	if targetPartner == "" {
		return nil, appErrors.BadRequest("partner_id is required")
	}
	partnerUUID, err := uuid.Parse(targetPartner)
	if err != nil {
		return nil, appErrors.BadRequest("invalid partner_id uuid format")
	}
	return s.repo.GetPartnerProfile(ctx, partnerUUID)
}

func (s *deliveryService) UpdatePartnerProfile(ctx context.Context, partnerID string, companyName, email, contactNumber *string) (*model.DeliveryPartnerProfile, error) {
	targetPartner := strings.TrimSpace(partnerID)
	if targetPartner == "" {
		return nil, appErrors.BadRequest("partner_id is required")
	}
	partnerUUID, err := uuid.Parse(targetPartner)
	if err != nil {
		return nil, appErrors.BadRequest("invalid partner_id uuid format")
	}

	existing, err := s.repo.GetPartnerProfile(ctx, partnerUUID)
	if err != nil {
		if appErrors.IsNotFound(err) {
			now := time.Now().UTC()
			existing = &model.DeliveryPartnerProfile{
				ID:        uuid.New(),
				PartnerID: partnerUUID,
				IsActive:  true,
				CreatedAt: now,
				UpdatedAt: now,
			}
		} else {
			return nil, err
		}
	}

	if companyName != nil {
		existing.CompanyName = strings.TrimSpace(*companyName)
	}
	if email != nil {
		trimmedEmail := strings.TrimSpace(*email)
		if trimmedEmail == "" {
			return nil, appErrors.BadRequest("email cannot be empty")
		}
		existing.Email = trimmedEmail
	}
	if contactNumber != nil {
		existing.ContactNumber = strings.TrimSpace(*contactNumber)
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := s.repo.CreateOrUpdatePartnerProfile(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// resolveProvisionalWarehouse selects an available origin warehouse for preliminary transit calculation.
// Checks warehouse availability (is_active) and prioritizes proximity (same-city, then same-state).
func resolveProvisionalWarehouse(warehouses []*model.Warehouse, destCity, destState string) *model.Warehouse {
	if len(warehouses) == 0 {
		return nil
	}
	dCity := strings.TrimSpace(strings.ToLower(destCity))
	dState := strings.TrimSpace(strings.ToLower(destState))

	// Filter for active/available warehouses first
	var activeWarehouses []*model.Warehouse
	for _, w := range warehouses {
		if w.IsActive {
			activeWarehouses = append(activeWarehouses, w)
		}
	}
	if len(activeWarehouses) == 0 {
		activeWarehouses = warehouses
	}

	if dCity != "" {
		for _, w := range activeWarehouses {
			if strings.TrimSpace(strings.ToLower(w.City)) == dCity {
				return w
			}
		}
	}
	if dState != "" {
		for _, w := range activeWarehouses {
			if strings.TrimSpace(strings.ToLower(w.State)) == dState {
				return w
			}
		}
	}
	return activeWarehouses[0]
}

// resolveProvisionalOrigin selects origin city and state from warehouses for backward compatibility.
func resolveProvisionalOrigin(warehouses []*model.Warehouse, destCity, destState string) (string, string) {
	w := resolveProvisionalWarehouse(warehouses, destCity, destState)
	if w == nil {
		return "", ""
	}
	return w.City, w.State
}
