package grpc

import (
	"context"
	"time"

	inventorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/service"
)

type InventoryGRPCServer struct {
	inventorypb.UnimplementedInventoryServiceServer
	service service.InventoryService
}

func NewInventoryGRPCServer(service service.InventoryService) *InventoryGRPCServer {
	return &InventoryGRPCServer{
		service: service,
	}
}

func toProtoInventoryItem(inv *model.Inventory) *inventorypb.InventoryItem {
	if inv == nil {
		return nil
	}
	var variantID string
	if inv.VariantID != nil {
		variantID = *inv.VariantID
	}
	var warehouseID string
	if inv.WarehouseID != nil {
		warehouseID = *inv.WarehouseID
	}
	return &inventorypb.InventoryItem{
		InventoryId:       inv.ID,
		WarehouseId:       warehouseID,
		ProductId:         inv.ProductID,
		VariantId:         variantID,
		Sku:               inv.SKU,
		AvailableQuantity: int32(inv.AvailableQuantity),
		ReservedQuantity:  int32(inv.ReservedQuantity),
		LowStockThreshold: int32(inv.LowStockThreshold),
		UpdatedAt:         inv.UpdatedAt.UTC().Format(time.RFC3339),
		CreatedAt:         inv.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *InventoryGRPCServer) CreateInventory(ctx context.Context, req *inventorypb.CreateInventoryRequest) (*inventorypb.CreateInventoryResponse, error) {
	if req == nil {
		return nil, appErrors.BadRequest("request cannot be nil").ToGRPC()
	}

	var variantID *string
	if req.GetVariantId() != "" {
		v := req.GetVariantId()
		variantID = &v
	}

	var warehouseID *string
	if req.GetWarehouseId() != "" {
		w := req.GetWarehouseId()
		warehouseID = &w
	}

	inv, err := s.service.CreateInventory(ctx, dto.CreateInventoryInput{
		MerchantID:        req.GetMerchantId(),
		WarehouseID:       warehouseID,
		ProductID:         req.GetProductId(),
		VariantID:         variantID,
		SKU:               req.GetSku(),
		InitialQuantity:   int(req.GetInitialQuantity()),
		LowStockThreshold: int(req.GetLowStockThreshold()),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &inventorypb.CreateInventoryResponse{
		Inventory: toProtoInventoryItem(inv),
	}, nil
}

func (s *InventoryGRPCServer) GetInventory(ctx context.Context, req *inventorypb.GetInventoryRequest) (*inventorypb.GetInventoryResponse, error) {
	if req == nil {
		return nil, appErrors.BadRequest("request cannot be nil").ToGRPC()
	}

	var invID, prodID, varID, merchID, whID *string
	if req.GetInventoryId() != "" {
		v := req.GetInventoryId()
		invID = &v
	}
	if req.GetWarehouseId() != "" {
		v := req.GetWarehouseId()
		whID = &v
	}
	if req.GetProductId() != "" {
		v := req.GetProductId()
		prodID = &v
	}
	if req.GetVariantId() != "" {
		v := req.GetVariantId()
		varID = &v
	}
	if req.GetMerchantId() != "" {
		v := req.GetMerchantId()
		merchID = &v
	}

	inv, err := s.service.GetInventory(ctx, dto.GetInventoryInput{
		InventoryID: invID,
		WarehouseID: whID,
		ProductID:   prodID,
		VariantID:   varID,
		MerchantID:  merchID,
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &inventorypb.GetInventoryResponse{
		Inventory: toProtoInventoryItem(inv),
	}, nil
}

func (s *InventoryGRPCServer) RestockInventory(ctx context.Context, req *inventorypb.RestockInventoryRequest) (*inventorypb.RestockInventoryResponse, error) {
	if req == nil {
		return nil, appErrors.BadRequest("request cannot be nil").ToGRPC()
	}

	var invID, prodID, varID, refID, notes *string
	if req.GetInventoryId() != "" {
		v := req.GetInventoryId()
		invID = &v
	}
	if req.GetProductId() != "" {
		v := req.GetProductId()
		prodID = &v
	}
	if req.GetVariantId() != "" {
		v := req.GetVariantId()
		varID = &v
	}
	if req.GetReferenceId() != "" {
		v := req.GetReferenceId()
		refID = &v
	}
	if req.GetNotes() != "" {
		v := req.GetNotes()
		notes = &v
	}

	inv, err := s.service.RestockInventory(ctx, dto.RestockInventoryInput{
		MerchantID:  req.GetMerchantId(),
		InventoryID: invID,
		ProductID:   prodID,
		VariantID:   varID,
		Quantity:    int(req.GetQuantity()),
		ReferenceID: refID,
		Notes:       notes,
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &inventorypb.RestockInventoryResponse{
		Inventory: toProtoInventoryItem(inv),
		Success:   true,
	}, nil
}

func (s *InventoryGRPCServer) GetStock(ctx context.Context, req *inventorypb.GetStockRequest) (*inventorypb.GetStockResponse, error) {
	if req == nil || req.GetProductId() == "" {
		return nil, appErrors.BadRequest("product_id is required").ToGRPC()
	}

	inv, err := s.service.GetStock(ctx, req.GetProductId())
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &inventorypb.GetStockResponse{
		Stock: &inventorypb.StockItem{
			ProductId:         inv.ProductID,
			AvailableQuantity: int32(inv.AvailableQuantity),
			ReservedQuantity:  int32(inv.ReservedQuantity),
		},
	}, nil
}

func (s *InventoryGRPCServer) ReserveStock(ctx context.Context, req *inventorypb.ReserveStockRequest) (*inventorypb.ReserveStockResponse, error) {
	if req == nil {
		return nil, appErrors.BadRequest("request cannot be nil").ToGRPC()
	}

	var items []dto.ReserveItemInput
	for _, it := range req.GetItems() {
		var variantID *string
		if it.GetVariantId() != "" {
			v := it.GetVariantId()
			variantID = &v
		}
		items = append(items, dto.ReserveItemInput{
			ProductID: it.GetProductId(),
			VariantID: variantID,
			Quantity:  int(it.GetQuantity()),
		})
	}

	resID, expiresAt, err := s.service.ReserveStock(ctx, dto.ReserveStockInput{
		OrderID:           req.GetOrderId(),
		Items:             items,
		ExpirationMinutes: int(req.GetExpirationMinutes()),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &inventorypb.ReserveStockResponse{
		ReservationId: resID,
		Success:       true,
		ExpiresAt:     expiresAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *InventoryGRPCServer) ReleaseStock(ctx context.Context, req *inventorypb.ReleaseStockRequest) (*inventorypb.ReleaseStockResponse, error) {
	if req == nil || (req.GetReservationId() == "" && req.GetOrderId() == "") {
		return nil, appErrors.BadRequest("either reservation_id or order_id is required").ToGRPC()
	}

	err := s.service.ReleaseStock(ctx, dto.ReleaseStockInput{
		ReservationID: req.GetReservationId(),
		OrderID:       req.GetOrderId(),
		Reason:        req.GetReason(),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &inventorypb.ReleaseStockResponse{
		Success: true,
	}, nil
}

func (s *InventoryGRPCServer) ReleaseExpiredReservations(ctx context.Context, req *inventorypb.ReleaseExpiredReservationsRequest) (*inventorypb.ReleaseExpiredReservationsResponse, error) {
	count, err := s.service.ReleaseExpiredReservations(ctx)
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}
	return &inventorypb.ReleaseExpiredReservationsResponse{
		ReleasedCount: int32(count),
	}, nil
}

func (s *InventoryGRPCServer) UpdateStock(ctx context.Context, req *inventorypb.UpdateStockRequest) (*inventorypb.UpdateStockResponse, error) {
	if req == nil || (req.GetInventoryId() == "" && req.GetProductId() == "") {
		return nil, appErrors.BadRequest("either inventory_id or product_id is required").ToGRPC()
	}

	var invID, prodID, varID *string
	if req.GetInventoryId() != "" {
		v := req.GetInventoryId()
		invID = &v
	}
	if req.GetProductId() != "" {
		v := req.GetProductId()
		prodID = &v
	}
	if req.GetVariantId() != "" {
		v := req.GetVariantId()
		varID = &v
	}

	inv, err := s.service.UpdateStock(ctx, dto.UpdateStockInput{
		InventoryID: invID,
		ProductID:   prodID,
		VariantID:   varID,
		Quantity:    int(req.GetQuantity()),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &inventorypb.UpdateStockResponse{
		Stock: &inventorypb.StockItem{
			ProductId:         inv.ProductID,
			AvailableQuantity: int32(inv.AvailableQuantity),
			ReservedQuantity:  int32(inv.ReservedQuantity),
		},
	}, nil
}

func (s *InventoryGRPCServer) CommitStock(ctx context.Context, req *inventorypb.CommitStockRequest) (*inventorypb.CommitStockResponse, error) {
	if req == nil || (req.GetReservationId() == "" && req.GetOrderId() == "") {
		return nil, appErrors.BadRequest("either reservation_id or order_id is required").ToGRPC()
	}

	err := s.service.CommitStock(ctx, dto.CommitStockInput{
		ReservationID: req.GetReservationId(),
		OrderID:       req.GetOrderId(),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &inventorypb.CommitStockResponse{
		Success: true,
	}, nil
}

func toProtoWarehouse(w *model.Warehouse) *inventorypb.Warehouse {
	if w == nil {
		return nil
	}
	return &inventorypb.Warehouse{
		Id:                w.ID.String(),
		DeliveryPartnerId: w.DeliveryPartnerID.String(),
		Name:              w.Name,
		Code:              w.Code,
		Street:            w.Street,
		City:              w.City,
		State:             w.State,
		PostalCode:        w.PostalCode,
		IsActive:          w.IsActive,
		CreatedAt:         w.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:         w.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *InventoryGRPCServer) CreateWarehouse(ctx context.Context, req *inventorypb.CreateWarehouseRequest) (*inventorypb.WarehouseResponse, error) {
	if req == nil {
		return nil, appErrors.BadRequest("request cannot be nil").ToGRPC()
	}

	isActive := req.GetIsActive()
	wh, err := s.service.CreateWarehouse(ctx, req.GetDeliveryPartnerId(), &dto.CreateWarehouseRequest{
		DeliveryPartnerID: req.GetDeliveryPartnerId(),
		Name:              req.GetName(),
		Code:              req.GetCode(),
		Street:            req.GetStreet(),
		City:              req.GetCity(),
		State:             req.GetState(),
		PostalCode:        req.GetPostalCode(),
		IsActive:          &isActive,
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &inventorypb.WarehouseResponse{
		Warehouse: toProtoWarehouse(wh),
	}, nil
}

func (s *InventoryGRPCServer) UpdateWarehouse(ctx context.Context, req *inventorypb.UpdateWarehouseRequest) (*inventorypb.WarehouseResponse, error) {
	if req == nil {
		return nil, appErrors.BadRequest("request cannot be nil").ToGRPC()
	}

	updateReq := &dto.UpdateWarehouseRequest{
		DeliveryPartnerID: req.GetDeliveryPartnerId(),
		ID:                req.GetId(),
		Name:              req.Name,
		Code:              req.Code,
		Street:            req.Street,
		City:              req.City,
		State:             req.State,
		PostalCode:        req.PostalCode,
		IsActive:          req.IsActive,
	}

	wh, err := s.service.UpdateWarehouse(ctx, req.GetDeliveryPartnerId(), req.GetId(), updateReq)
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &inventorypb.WarehouseResponse{
		Warehouse: toProtoWarehouse(wh),
	}, nil
}

func (s *InventoryGRPCServer) DeleteWarehouse(ctx context.Context, req *inventorypb.DeleteWarehouseRequest) (*inventorypb.DeleteWarehouseResponse, error) {
	if req == nil {
		return nil, appErrors.BadRequest("request cannot be nil").ToGRPC()
	}

	if err := s.service.DeleteWarehouse(ctx, req.GetDeliveryPartnerId(), req.GetId()); err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &inventorypb.DeleteWarehouseResponse{
		Success: true,
		Message: "Warehouse deleted successfully",
	}, nil
}

func (s *InventoryGRPCServer) GetWarehouse(ctx context.Context, req *inventorypb.GetWarehouseRequest) (*inventorypb.WarehouseResponse, error) {
	if req == nil {
		return nil, appErrors.BadRequest("request cannot be nil").ToGRPC()
	}

	wh, err := s.service.GetWarehouseByID(ctx, req.GetDeliveryPartnerId(), req.GetId())
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &inventorypb.WarehouseResponse{
		Warehouse: toProtoWarehouse(wh),
	}, nil
}

func (s *InventoryGRPCServer) ListWarehouses(ctx context.Context, req *inventorypb.ListWarehousesRequest) (*inventorypb.ListWarehousesResponse, error) {
	partnerID := ""
	if req != nil {
		partnerID = req.GetDeliveryPartnerId()
	}

	var warehouses []*model.Warehouse
	var err error
	if partnerID != "" {
		warehouses, err = s.service.GetPartnerWarehouses(ctx, partnerID)
	} else {
		warehouses, err = s.service.GetAvailableWarehouses(ctx)
	}
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	protoWarehouses := make([]*inventorypb.Warehouse, len(warehouses))
	for i, w := range warehouses {
		protoWarehouses[i] = toProtoWarehouse(w)
	}

	return &inventorypb.ListWarehousesResponse{
		Warehouses: protoWarehouses,
	}, nil
}
