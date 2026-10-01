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
}

func (m *mockInventoryClientForResolvers) CreateInventory(ctx context.Context, in *inventorypb.CreateInventoryRequest, opts ...grpcPkg.CallOption) (*inventorypb.CreateInventoryResponse, error) {
	m.lastCreateReq = in
	return &inventorypb.CreateInventoryResponse{
		Inventory: &inventorypb.InventoryItem{
			InventoryId:       "inv-123",
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
	res, err := r.CreateInventory(ctx, model.CreateInventoryInput{
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
	if invMock.lastCreateReq.MerchantId != "merch-1" {
		t.Errorf("expected merchantId merch-1, got %s", invMock.lastCreateReq.MerchantId)
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
	res, err := r.Inventory(ctx, &prodID, nil, nil)
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
