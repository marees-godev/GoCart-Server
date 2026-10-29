package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/model"
)

func (s *inventoryService) CreateWarehouse(ctx context.Context, partnerID string, req *dto.CreateWarehouseRequest) (*model.Warehouse, error) {
	if s.whRepo == nil {
		return nil, appErrors.Internal(nil, "warehouse repository is not configured")
	}

	targetPartner := strings.TrimSpace(partnerID)
	if targetPartner == "" && req != nil && strings.TrimSpace(req.DeliveryPartnerID) != "" {
		targetPartner = strings.TrimSpace(req.DeliveryPartnerID)
	}
	if targetPartner == "" {
		return nil, appErrors.BadRequest("delivery_partner_id is required")
	}

	partnerUUID, err := uuid.Parse(targetPartner)
	if err != nil {
		return nil, appErrors.BadRequest("invalid delivery_partner_id uuid format")
	}

	if req == nil {
		return nil, appErrors.BadRequest("warehouse payload is required")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, appErrors.BadRequest("warehouse name is required")
	}

	code := strings.TrimSpace(req.Code)
	if code == "" {
		return nil, appErrors.BadRequest("warehouse code is required")
	}

	city := strings.TrimSpace(req.City)
	if city == "" {
		return nil, appErrors.BadRequest("city is required")
	}

	state := strings.TrimSpace(req.State)
	if state == "" {
		return nil, appErrors.BadRequest("state is required")
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	now := time.Now().UTC()
	warehouse := &model.Warehouse{
		ID:                uuid.New(),
		DeliveryPartnerID: partnerUUID,
		Name:              name,
		Code:              code,
		Street:            strings.TrimSpace(req.Street),
		City:              city,
		State:             state,
		PostalCode:        strings.TrimSpace(req.PostalCode),
		IsActive:          isActive,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.whRepo.CreateWarehouse(ctx, warehouse); err != nil {
		return nil, err
	}

	return warehouse, nil
}

func (s *inventoryService) UpdateWarehouse(ctx context.Context, partnerID string, warehouseID string, req *dto.UpdateWarehouseRequest) (*model.Warehouse, error) {
	if s.whRepo == nil {
		return nil, appErrors.Internal(nil, "warehouse repository is not configured")
	}

	targetPartner := strings.TrimSpace(partnerID)
	var partnerUUID uuid.UUID
	var err error
	if targetPartner != "" {
		partnerUUID, err = uuid.Parse(targetPartner)
		if err != nil {
			return nil, appErrors.BadRequest("invalid delivery_partner_id uuid format")
		}
	}

	targetWarehouse := strings.TrimSpace(warehouseID)
	if targetWarehouse == "" {
		return nil, appErrors.BadRequest("warehouse_id is required")
	}
	warehouseUUID, err := uuid.Parse(targetWarehouse)
	if err != nil {
		return nil, appErrors.BadRequest("invalid warehouse_id uuid format")
	}

	existing, err := s.whRepo.GetWarehouseByID(ctx, partnerUUID, warehouseUUID)
	if err != nil {
		return nil, err
	}

	if req != nil {
		if req.Name != nil {
			existing.Name = strings.TrimSpace(*req.Name)
		}
		if req.Code != nil {
			existing.Code = strings.TrimSpace(*req.Code)
		}
		if req.Street != nil {
			existing.Street = strings.TrimSpace(*req.Street)
		}
		if req.City != nil {
			existing.City = strings.TrimSpace(*req.City)
		}
		if req.State != nil {
			existing.State = strings.TrimSpace(*req.State)
		}
		if req.PostalCode != nil {
			existing.PostalCode = strings.TrimSpace(*req.PostalCode)
		}
		if req.IsActive != nil {
			existing.IsActive = *req.IsActive
		}
	}
	existing.UpdatedAt = time.Now().UTC()

	return s.whRepo.UpdateWarehouse(ctx, existing)
}

func (s *inventoryService) DeleteWarehouse(ctx context.Context, partnerID string, warehouseID string) error {
	if s.whRepo == nil {
		return appErrors.Internal(nil, "warehouse repository is not configured")
	}

	targetPartner := strings.TrimSpace(partnerID)
	var partnerUUID uuid.UUID
	var err error
	if targetPartner != "" {
		partnerUUID, err = uuid.Parse(targetPartner)
		if err != nil {
			return appErrors.BadRequest("invalid delivery_partner_id uuid format")
		}
	}

	targetWarehouse := strings.TrimSpace(warehouseID)
	if targetWarehouse == "" {
		return appErrors.BadRequest("warehouse_id is required")
	}
	warehouseUUID, err := uuid.Parse(targetWarehouse)
	if err != nil {
		return appErrors.BadRequest("invalid warehouse_id uuid format")
	}

	return s.whRepo.DeleteWarehouse(ctx, partnerUUID, warehouseUUID)
}

func (s *inventoryService) GetWarehouseByID(ctx context.Context, partnerID string, warehouseID string) (*model.Warehouse, error) {
	if s.whRepo == nil {
		return nil, appErrors.Internal(nil, "warehouse repository is not configured")
	}

	targetWarehouse := strings.TrimSpace(warehouseID)
	if targetWarehouse == "" {
		return nil, appErrors.BadRequest("warehouse_id is required")
	}
	warehouseUUID, err := uuid.Parse(targetWarehouse)
	if err != nil {
		return nil, appErrors.BadRequest("invalid warehouse_id uuid format")
	}

	targetPartner := strings.TrimSpace(partnerID)
	var partnerUUID uuid.UUID
	if targetPartner != "" {
		partnerUUID, err = uuid.Parse(targetPartner)
		if err != nil {
			return nil, appErrors.BadRequest("invalid delivery_partner_id uuid format")
		}
	}

	return s.whRepo.GetWarehouseByID(ctx, partnerUUID, warehouseUUID)
}

func (s *inventoryService) GetPartnerWarehouses(ctx context.Context, partnerID string) ([]*model.Warehouse, error) {
	if s.whRepo == nil {
		return nil, appErrors.Internal(nil, "warehouse repository is not configured")
	}

	targetPartner := strings.TrimSpace(partnerID)
	var partnerUUID uuid.UUID
	var err error
	if targetPartner != "" {
		partnerUUID, err = uuid.Parse(targetPartner)
		if err != nil {
			return nil, appErrors.BadRequest("invalid delivery_partner_id uuid format")
		}
	}

	return s.whRepo.GetWarehousesByPartnerID(ctx, partnerUUID)
}

func (s *inventoryService) GetAvailableWarehouses(ctx context.Context) ([]*model.Warehouse, error) {
	if s.whRepo == nil {
		return nil, appErrors.Internal(nil, "warehouse repository is not configured")
	}

	return s.whRepo.GetAvailableWarehouses(ctx)
}
