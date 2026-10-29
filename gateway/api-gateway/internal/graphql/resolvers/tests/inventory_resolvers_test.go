package tests

import (
	"context"
	"testing"

	inventorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	grpcPkg "google.golang.org/grpc"
)

type mockInventoryClientForResolvers struct {
	inventorypb.InventoryServiceClient
	lastCreateReq  *inventorypb.CreateInventoryRequest
	lastRestockReq *inventorypb.RestockInventoryRequest
	lastGetReq     *inventorypb.GetInventoryRequest
	warehouses     map[string]*inventorypb.Warehouse
}

func (m *mockInventoryClientForResolvers) CreateWarehouse(ctx context.Context, in *inventorypb.CreateWarehouseRequest, opts ...grpcPkg.CallOption) (*inventorypb.WarehouseResponse, error) {
	if m.warehouses == nil {
		m.warehouses = make(map[string]*inventorypb.Warehouse)
	}
	wh := &inventorypb.Warehouse{
		Id:                "wh-123",
		DeliveryPartnerId: in.DeliveryPartnerId,
		Name:              in.Name,
		Code:              in.Code,
		Street:            in.Street,
		City:              in.City,
		State:             in.State,
		PostalCode:        in.PostalCode,
		IsActive:          in.IsActive,
		CreatedAt:         "2026-10-27T12:00:00Z",
		UpdatedAt:         "2026-10-27T12:00:00Z",
	}
	m.warehouses[wh.Id] = wh
	return &inventorypb.WarehouseResponse{Warehouse: wh}, nil
}

func (m *mockInventoryClientForResolvers) UpdateWarehouse(ctx context.Context, in *inventorypb.UpdateWarehouseRequest, opts ...grpcPkg.CallOption) (*inventorypb.WarehouseResponse, error) {
	wh, ok := m.warehouses[in.Id]
	if !ok {
		wh = &inventorypb.Warehouse{
			Id:                in.Id,
			DeliveryPartnerId: in.DeliveryPartnerId,
		}
	}
	if in.Name != nil {
		wh.Name = *in.Name
	}
	if in.Code != nil {
		wh.Code = *in.Code
	}
	if in.City != nil {
		wh.City = *in.City
	}
	m.warehouses[in.Id] = wh
	return &inventorypb.WarehouseResponse{Warehouse: wh}, nil
}

func (m *mockInventoryClientForResolvers) DeleteWarehouse(ctx context.Context, in *inventorypb.DeleteWarehouseRequest, opts ...grpcPkg.CallOption) (*inventorypb.DeleteWarehouseResponse, error) {
	delete(m.warehouses, in.Id)
	return &inventorypb.DeleteWarehouseResponse{Success: true, Message: "Warehouse deleted successfully"}, nil
}

func (m *mockInventoryClientForResolvers) GetWarehouse(ctx context.Context, in *inventorypb.GetWarehouseRequest, opts ...grpcPkg.CallOption) (*inventorypb.WarehouseResponse, error) {
	wh, ok := m.warehouses[in.Id]
	if !ok {
		wh = &inventorypb.Warehouse{
			Id:                in.Id,
			DeliveryPartnerId: in.DeliveryPartnerId,
			Name:              "Default Warehouse",
			City:              "Austin",
			State:             "TX",
		}
	}
	return &inventorypb.WarehouseResponse{Warehouse: wh}, nil
}

func (m *mockInventoryClientForResolvers) ListWarehouses(ctx context.Context, in *inventorypb.ListWarehousesRequest, opts ...grpcPkg.CallOption) (*inventorypb.ListWarehousesResponse, error) {
	var list []*inventorypb.Warehouse
	for _, wh := range m.warehouses {
		if wh.DeliveryPartnerId == in.DeliveryPartnerId {
			list = append(list, wh)
		}
	}
	return &inventorypb.ListWarehousesResponse{Warehouses: list}, nil
}

func (m *mockInventoryClientForResolvers) CreateInventory(ctx context.Context, in *inventorypb.CreateInventoryRequest, opts ...grpcPkg.CallOption) (*inventorypb.CreateInventoryResponse, error) {
	m.lastCreateReq = in
	return &inventorypb.CreateInventoryResponse{
		Inventory: &inventorypb.InventoryItem{
			InventoryId:       "inv-123",
			WarehouseId:       in.WarehouseId,
			ProductId:         in.ProductId,
			VariantId:         in.VariantId,
			Sku:               in.Sku,
			AvailableQuantity: in.InitialQuantity,
			ReservedQuantity:  0,
			LowStockThreshold: in.LowStockThreshold,
			UpdatedAt:         "2026-09-29T00:00:00Z",
			CreatedAt:         "2026-09-29T00:00:00Z",
		},
	}, nil
}

func (m *mockInventoryClientForResolvers) RestockInventory(ctx context.Context, in *inventorypb.RestockInventoryRequest, opts ...grpcPkg.CallOption) (*inventorypb.RestockInventoryResponse, error) {
	m.lastRestockReq = in
	return &inventorypb.RestockInventoryResponse{
		Inventory: &inventorypb.InventoryItem{
			InventoryId:       in.InventoryId,
			ProductId:         in.ProductId,
			VariantId:         in.VariantId,
			Sku:               "SKU-RESTOCKED",
			AvailableQuantity: in.Quantity + 20,
			ReservedQuantity:  0,
			LowStockThreshold: 5,
			UpdatedAt:         "2026-09-29T00:00:00Z",
			CreatedAt:         "2026-09-29T00:00:00Z",
		},
		Success: true,
	}, nil
}

func (m *mockInventoryClientForResolvers) GetInventory(ctx context.Context, in *inventorypb.GetInventoryRequest, opts ...grpcPkg.CallOption) (*inventorypb.GetInventoryResponse, error) {
	m.lastGetReq = in
	return &inventorypb.GetInventoryResponse{
		Inventory: &inventorypb.InventoryItem{
			InventoryId:       "inv-123",
			ProductId:         in.ProductId,
			VariantId:         in.VariantId,
			Sku:               "SKU-FETCHED",
			AvailableQuantity: 50,
			ReservedQuantity:  5,
			LowStockThreshold: 5,
			UpdatedAt:         "2026-09-29T00:00:00Z",
			CreatedAt:         "2026-09-29T00:00:00Z",
		},
	}, nil
}

func TestInventoryResolver_CreateInventory(t *testing.T) {
	invMock := &mockInventoryClientForResolvers{}
	clients := &grpc.Clients{
		InventoryClient: invMock,
	}

	r := (&resolvers.Resolver{
		Clients: clients,
	}).Mutation()

	userCtx := &auth.UserContext{
		UserID: "merch-1",
		Role:   "MERCHANT",
	}
	ctx := auth.WithUser(context.Background(), userCtx)

	threshold := 10
	variant := "var-1"
	sku := "SKU-1"
	whID := "wh-test-99"
	res, err := r.CreateInventory(ctx, model.CreateInventoryInput{
		WarehouseID:       &whID,
		ProductID:         "prod-1",
		VariantID:         &variant,
		Sku:               &sku,
		InitialQuantity:   100,
		LowStockThreshold: &threshold,
	})
	if err != nil {
		t.Fatalf("CreateInventory resolver failed: %v", err)
	}

	if res.InventoryID != "inv-123" {
		t.Errorf("expected inventoryId inv-123, got %s", res.InventoryID)
	}
	if res.WarehouseID == nil || *res.WarehouseID != whID {
		t.Errorf("expected warehouseId %s, got %v", whID, res.WarehouseID)
	}
	if invMock.lastCreateReq.MerchantId != "merch-1" {
		t.Errorf("expected merchantId merch-1, got %s", invMock.lastCreateReq.MerchantId)
	}
	if invMock.lastCreateReq.WarehouseId != whID {
		t.Errorf("expected proto warehouseId %s, got %s", whID, invMock.lastCreateReq.WarehouseId)
	}
}

func TestInventoryResolver_RestockInventory(t *testing.T) {
	invMock := &mockInventoryClientForResolvers{}
	clients := &grpc.Clients{
		InventoryClient: invMock,
	}

	r := (&resolvers.Resolver{
		Clients: clients,
	}).Mutation()

	userCtx := &auth.UserContext{
		UserID: "merch-1",
		Role:   "MERCHANT",
	}
	ctx := auth.WithUser(context.Background(), userCtx)

	invID := "inv-123"
	res, err := r.RestockInventory(ctx, model.RestockInventoryInput{
		InventoryID: &invID,
		Quantity:    30,
	})
	if err != nil {
		t.Fatalf("RestockInventory resolver failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected success true")
	}
	if res.Inventory.AvailableQuantity != 50 {
		t.Errorf("expected available quantity 50, got %d", res.Inventory.AvailableQuantity)
	}
}

func TestInventoryResolver_QueryInventory(t *testing.T) {
	invMock := &mockInventoryClientForResolvers{}
	clients := &grpc.Clients{
		InventoryClient: invMock,
	}

	r := (&resolvers.Resolver{
		Clients: clients,
	}).Query()

	userCtx := &auth.UserContext{
		UserID: "merch-1",
		Role:   "MERCHANT",
	}
	ctx := auth.WithUser(context.Background(), userCtx)

	prodID := "prod-1"
	res, err := r.Inventory(ctx, &prodID, nil, nil, nil)
	if err != nil {
		t.Fatalf("Inventory query resolver failed: %v", err)
	}

	if res.ProductID != "prod-1" {
		t.Errorf("expected product id prod-1, got %s", res.ProductID)
	}
	if res.AvailableQuantity != 50 {
		t.Errorf("expected available quantity 50, got %d", res.AvailableQuantity)
	}
}

func TestInventoryResolver_WarehouseCRUD(t *testing.T) {
	invMock := &mockInventoryClientForResolvers{}
	clients := &grpc.Clients{
		InventoryClient: invMock,
	}

	res := &resolvers.Resolver{
		Clients: clients,
	}

	partnerID := "partner-uuid-777"
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:    partnerID,
		Role:      "DELIVERY_PARTNER",
		PartnerID: partnerID,
	})

	// 1. CreateWarehouse
	created, err := res.Mutation().CreateWarehouse(ctx, model.CreateWarehouseInput{
		DeliveryPartnerID: partnerID,
		Name:              "Austin Hub",
		Code:              "WH-ATX-01",
		City:              "Austin",
		State:             "TX",
	})
	if err != nil {
		t.Fatalf("CreateWarehouse failed: %v", err)
	}
	if created.ID != "wh-123" {
		t.Errorf("expected wh-123, got %s", created.ID)
	}

	// 2. Query Warehouse
	wh, err := res.Query().Warehouse(ctx, "wh-123", &partnerID)
	if err != nil {
		t.Fatalf("Query Warehouse failed: %v", err)
	}
	if wh.ID != "wh-123" {
		t.Errorf("expected wh-123, got %s", wh.ID)
	}

	// 3. Update Warehouse
	newName := "Austin Central Hub"
	updated, err := res.Mutation().UpdateWarehouse(ctx, model.UpdateWarehouseInput{
		ID:                "wh-123",
		DeliveryPartnerID: partnerID,
		Name:              &newName,
	})
	if err != nil {
		t.Fatalf("UpdateWarehouse failed: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("expected name %s, got %s", newName, updated.Name)
	}

	// 4. List Warehouses
	list, err := res.Query().MyWarehouses(ctx, &partnerID)
	if err != nil {
		t.Fatalf("MyWarehouses failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 warehouse, got %d", len(list))
	}

	// 5. Delete Warehouse
	delRes, err := res.Mutation().DeleteWarehouse(ctx, "wh-123", &partnerID)
	if err != nil {
		t.Fatalf("DeleteWarehouse failed: %v", err)
	}
	if !delRes.Success {
		t.Errorf("expected success true, got false")
	}
}
