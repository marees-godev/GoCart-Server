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
	return &inventorypb.InventoryItem{
		InventoryId:       inv.ID,
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

	inv, err := s.service.CreateInventory(ctx, dto.CreateInventoryInput{
		MerchantID:        req.GetMerchantId(),
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

	var invID, prodID, varID, merchID *string
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
	if req.GetMerchantId() != "" {
		v := req.GetMerchantId()
		merchID = &v
	}

	inv, err := s.service.GetInventory(ctx, dto.GetInventoryInput{
		InventoryID: invID,
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
