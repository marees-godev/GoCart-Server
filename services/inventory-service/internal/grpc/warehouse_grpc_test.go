package grpc_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	inventorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	inventoryGRPC "github.com/marees-godev/GoCart-Server/services/inventory-service/internal/grpc"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type memoryWarehouseRepo struct {
	mu         sync.RWMutex
	warehouses map[uuid.UUID]*model.Warehouse
}

func newMemoryWarehouseRepo() repository.WarehouseRepository {
	return &memoryWarehouseRepo{
		warehouses: make(map[uuid.UUID]*model.Warehouse),
	}
}

func (r *memoryWarehouseRepo) CreateWarehouse(ctx context.Context, w *model.Warehouse) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.warehouses {
		if existing.DeliveryPartnerID == w.DeliveryPartnerID && existing.Code == w.Code {
			return appErrors.Conflict("warehouse code already exists for this delivery partner")
		}
	}

	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	now := time.Now().UTC()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	if w.UpdatedAt.IsZero() {
		w.UpdatedAt = now
	}
	r.warehouses[w.ID] = w
	return nil
}

func (r *memoryWarehouseRepo) GetWarehouseByID(ctx context.Context, partnerID, id uuid.UUID) (*model.Warehouse, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	w, ok := r.warehouses[id]
	if !ok {
		return nil, appErrors.NotFound("warehouse not found")
	}
	if partnerID != uuid.Nil && w.DeliveryPartnerID != partnerID {
		return nil, appErrors.NotFound("warehouse not found")
	}
	return w, nil
}

func (r *memoryWarehouseRepo) GetWarehousesByPartnerID(ctx context.Context, partnerID uuid.UUID) ([]*model.Warehouse, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Warehouse
	for _, w := range r.warehouses {
		if partnerID == uuid.Nil || w.DeliveryPartnerID == partnerID {
			result = append(result, w)
		}
	}
	return result, nil
}

func (r *memoryWarehouseRepo) GetAvailableWarehouses(ctx context.Context) ([]*model.Warehouse, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Warehouse
	for _, w := range r.warehouses {
		if w.IsActive {
			result = append(result, w)
		}
	}
	return result, nil
}

func (r *memoryWarehouseRepo) UpdateWarehouse(ctx context.Context, w *model.Warehouse) (*model.Warehouse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.warehouses[w.ID]
	if !ok {
		return nil, appErrors.NotFound("warehouse not found")
	}
	if w.DeliveryPartnerID != uuid.Nil && existing.DeliveryPartnerID != w.DeliveryPartnerID {
		return nil, appErrors.NotFound("warehouse not found")
	}

	for id, other := range r.warehouses {
		if id != w.ID && other.DeliveryPartnerID == w.DeliveryPartnerID && other.Code == w.Code {
			return nil, appErrors.Conflict("warehouse code already exists for this delivery partner")
		}
	}

	w.UpdatedAt = time.Now().UTC()
	r.warehouses[w.ID] = w
	return w, nil
}

func (r *memoryWarehouseRepo) DeleteWarehouse(ctx context.Context, partnerID, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	w, ok := r.warehouses[id]
	if !ok {
		return appErrors.NotFound("warehouse not found")
	}
	if partnerID != uuid.Nil && w.DeliveryPartnerID != partnerID {
		return appErrors.NotFound("warehouse not found")
	}

	delete(r.warehouses, id)
	return nil
}

func setupWarehouseGRPCTestServer() (*inventoryGRPC.InventoryGRPCServer, repository.WarehouseRepository) {
	whRepo := newMemoryWarehouseRepo()
	svc := service.NewInventoryService(nil, nil, whRepo)
	server := inventoryGRPC.NewInventoryGRPCServer(svc)
	return server, whRepo
}

func TestInventoryGRPCServer_WarehouseCRUD(t *testing.T) {
	server, _ := setupWarehouseGRPCTestServer()
	ctx := context.Background()
	partnerID := uuid.NewString()

	// 1. CreateWarehouse
	createReq := &inventorypb.CreateWarehouseRequest{
		DeliveryPartnerId: partnerID,
		Name:              "Dallas Central Hub",
		Code:              "WH-DFW-01",
		Street:            "100 Main St",
		City:              "Dallas",
		State:             "TX",
		PostalCode:        "75001",
		IsActive:          true,
	}

	createResp, err := server.CreateWarehouse(ctx, createReq)
	if err != nil {
		t.Fatalf("CreateWarehouse failed: %v", err)
	}
	if createResp.Warehouse == nil || createResp.Warehouse.Id == "" {
		t.Fatalf("expected created warehouse with id")
	}
	warehouseID := createResp.Warehouse.Id

	// 2. Duplicate code conflict
	_, err = server.CreateWarehouse(ctx, createReq)
	if err == nil {
		t.Fatalf("expected error for duplicate code, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.AlreadyExists {
		t.Errorf("expected AlreadyExists, got %v", st.Code())
	}

	// 3. GetWarehouse
	getResp, err := server.GetWarehouse(ctx, &inventorypb.GetWarehouseRequest{
		DeliveryPartnerId: partnerID,
		Id:                warehouseID,
	})
	if err != nil {
		t.Fatalf("GetWarehouse failed: %v", err)
	}
	if getResp.Warehouse.Name != "Dallas Central Hub" {
		t.Errorf("expected name 'Dallas Central Hub', got %s", getResp.Warehouse.Name)
	}

	// 4. Partner Isolation check
	otherPartnerID := uuid.NewString()
	_, err = server.GetWarehouse(ctx, &inventorypb.GetWarehouseRequest{
		DeliveryPartnerId: otherPartnerID,
		Id:                warehouseID,
	})
	if err == nil {
		t.Fatalf("expected NotFound error for other partner accessing warehouse, got nil")
	}

	// 5. UpdateWarehouse
	newName := "Dallas North Hub"
	updateResp, err := server.UpdateWarehouse(ctx, &inventorypb.UpdateWarehouseRequest{
		DeliveryPartnerId: partnerID,
		Id:                warehouseID,
		Name:              &newName,
	})
	if err != nil {
		t.Fatalf("UpdateWarehouse failed: %v", err)
	}
	if updateResp.Warehouse.Name != newName {
		t.Errorf("expected updated name %s, got %s", newName, updateResp.Warehouse.Name)
	}

	// 6. ListWarehouses for partner
	listResp, err := server.ListWarehouses(ctx, &inventorypb.ListWarehousesRequest{
		DeliveryPartnerId: partnerID,
	})
	if err != nil {
		t.Fatalf("ListWarehouses failed: %v", err)
	}
	if len(listResp.Warehouses) != 1 {
		t.Errorf("expected 1 warehouse, got %d", len(listResp.Warehouses))
	}

	// 7. DeleteWarehouse
	delResp, err := server.DeleteWarehouse(ctx, &inventorypb.DeleteWarehouseRequest{
		DeliveryPartnerId: partnerID,
		Id:                warehouseID,
	})
	if err != nil {
		t.Fatalf("DeleteWarehouse failed: %v", err)
	}
	if !delResp.Success {
		t.Errorf("expected success true, got false")
	}

	// 8. Confirm deleted
	_, err = server.GetWarehouse(ctx, &inventorypb.GetWarehouseRequest{
		DeliveryPartnerId: partnerID,
		Id:                warehouseID,
	})
	if err == nil {
		t.Fatalf("expected NotFound after delete, got nil")
	}
}

func TestInventoryGRPCServer_CreateInventory_WithWarehouseID(t *testing.T) {
	ctx := context.Background()
	invRepo := newMemoryInventoryRepo()
	whRepo := newMemoryWarehouseRepo()
	svc := service.NewInventoryService(invRepo, nil, whRepo)
	server := inventoryGRPC.NewInventoryGRPCServer(svc)

	whID := uuid.NewString()
	prodID := uuid.NewString()

	resp, err := server.CreateInventory(ctx, &inventorypb.CreateInventoryRequest{
		ProductId:       prodID,
		WarehouseId:     whID,
		InitialQuantity: 50,
		Sku:             "SKU-WH-TEST",
	})
	if err != nil {
		t.Fatalf("CreateInventory with WarehouseId failed: %v", err)
	}
	if resp.Inventory.WarehouseId != whID {
		t.Errorf("expected WarehouseId %s, got %s", whID, resp.Inventory.WarehouseId)
	}

	getResp, err := server.GetInventory(ctx, &inventorypb.GetInventoryRequest{
		InventoryId: resp.Inventory.InventoryId,
	})
	if err != nil {
		t.Fatalf("GetInventory failed: %v", err)
	}
	if getResp.Inventory.WarehouseId != whID {
		t.Errorf("expected fetched WarehouseId %s, got %s", whID, getResp.Inventory.WarehouseId)
	}
}
