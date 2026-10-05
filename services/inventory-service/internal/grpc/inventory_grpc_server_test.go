package grpc_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	inventorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/dto"
	inventoryGRPC "github.com/marees-godev/GoCart-Server/services/inventory-service/internal/grpc"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type memoryInventoryRepo struct {
	mu           sync.RWMutex
	inventories  map[string]*model.Inventory
	reservations map[string]*model.InventoryReservation
	transactions []*model.InventoryTransaction
}

func newMemoryInventoryRepo() repository.InventoryRepository {
	return &memoryInventoryRepo{
		inventories:  make(map[string]*model.Inventory),
		reservations: make(map[string]*model.InventoryReservation),
		transactions: make([]*model.InventoryTransaction, 0),
	}
}

func (r *memoryInventoryRepo) Create(ctx context.Context, inv *model.Inventory, initialQuantity int, notes string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.inventories {
		if existing.ProductID == inv.ProductID {
			if (existing.VariantID == nil && inv.VariantID == nil) ||
				(existing.VariantID != nil && inv.VariantID != nil && *existing.VariantID == *inv.VariantID) {
				return appErrors.Conflict("inventory already exists for this product/variant")
			}
		}
	}

	if inv.ID == "" {
		inv.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	inv.CreatedAt = now
	inv.UpdatedAt = now

	stored := *inv
	r.inventories[inv.ID] = &stored

	if initialQuantity > 0 {
		r.transactions = append(r.transactions, &model.InventoryTransaction{
			ID:          uuid.NewString(),
			InventoryID: inv.ID,
			Type:        model.TransactionTypeRestock,
			Quantity:    initialQuantity,
			Notes:       &notes,
			CreatedAt:   now,
		})
	}
	return nil
}

func (r *memoryInventoryRepo) GetByID(ctx context.Context, id string) (*model.Inventory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	inv, ok := r.inventories[id]
	if !ok {
		return nil, appErrors.NotFound("inventory not found")
	}
	copyInv := *inv
	return &copyInv, nil
}

func (r *memoryInventoryRepo) GetByProductAndVariant(ctx context.Context, productID string, variantID *string) (*model.Inventory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, inv := range r.inventories {
		if inv.ProductID == productID {
			if variantID == nil || *variantID == "" {
				if inv.VariantID == nil {
					copyInv := *inv
					return &copyInv, nil
				}
			} else if inv.VariantID != nil && *inv.VariantID == *variantID {
				copyInv := *inv
				return &copyInv, nil
			}
		}
	}
	return nil, appErrors.NotFound("inventory not found for product/variant")
}

func (r *memoryInventoryRepo) Restock(ctx context.Context, id string, quantity int, refID, notes *string) (*model.Inventory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	inv, ok := r.inventories[id]
	if !ok {
		return nil, appErrors.NotFound("inventory not found for restocking")
	}

	inv.AvailableQuantity += quantity
	inv.UpdatedAt = time.Now().UTC()

	r.transactions = append(r.transactions, &model.InventoryTransaction{
		ID:          uuid.NewString(),
		InventoryID: id,
		Type:        model.TransactionTypeRestock,
		Quantity:    quantity,
		ReferenceID: refID,
		Notes:       notes,
		CreatedAt:   inv.UpdatedAt,
	})

	copyInv := *inv
	return &copyInv, nil
}

func (r *memoryInventoryRepo) UpdateStock(ctx context.Context, input dto.UpdateStockInput) (*model.Inventory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var invID, prodID, varID string
	if input.InventoryID != nil {
		invID = *input.InventoryID
	}
	if input.ProductID != nil {
		prodID = *input.ProductID
	}
	if input.VariantID != nil {
		varID = *input.VariantID
	}

	for _, inv := range r.inventories {
		if invID != "" && inv.ID == invID {
			inv.AvailableQuantity = input.Quantity
			inv.UpdatedAt = time.Now().UTC()
			copyInv := *inv
			return &copyInv, nil
		}
		if prodID != "" && inv.ProductID == prodID {
			matches := false
			if varID == "" && inv.VariantID == nil {
				matches = true
			} else if varID != "" && inv.VariantID != nil && varID == *inv.VariantID {
				matches = true
			}
			if matches {
				inv.AvailableQuantity = input.Quantity
				inv.UpdatedAt = time.Now().UTC()
				copyInv := *inv
				return &copyInv, nil
			}
		}
	}
	return nil, appErrors.NotFound("inventory not found for update")
}

func (r *memoryInventoryRepo) ReserveStock(ctx context.Context, orderID string, items []model.ReserveItem, expiresAt time.Time) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	resID := uuid.NewString()
	now := time.Now().UTC()

	for _, item := range items {
		var found *model.Inventory
		for _, inv := range r.inventories {
			if inv.ProductID == item.ProductID {
				if (item.VariantID == nil && inv.VariantID == nil) ||
					(item.VariantID != nil && inv.VariantID != nil && *item.VariantID == *inv.VariantID) {
					found = inv
					break
				}
			}
		}
		if found == nil {
			return "", appErrors.NotFound(fmt.Sprintf("inventory not found for product %s", item.ProductID))
		}
		if found.AvailableQuantity < item.Quantity {
			return "", appErrors.Conflict("insufficient stock")
		}
		found.AvailableQuantity -= item.Quantity
		found.ReservedQuantity += item.Quantity
		found.UpdatedAt = now

		r.reservations[resID] = &model.InventoryReservation{
			ID:        resID,
			OrderID:   orderID,
			ProductID: item.ProductID,
			VariantID: item.VariantID,
			Quantity:  item.Quantity,
			Status:    model.ReservationStatusReserved,
			ExpiresAt: expiresAt,
			CreatedAt: now,
			UpdatedAt: now,
		}

		ref := orderID
		notes := fmt.Sprintf("Stock reserved for order %s", orderID)
		r.transactions = append(r.transactions, &model.InventoryTransaction{
			ID:          uuid.NewString(),
			InventoryID: found.ID,
			Type:        model.TransactionTypeReserve,
			Quantity:    item.Quantity,
			ReferenceID: &ref,
			Notes:       &notes,
			CreatedAt:   now,
		})
	}
	return resID, nil
}

func (r *memoryInventoryRepo) ReleaseStock(ctx context.Context, input dto.ReleaseStockInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var matching []*model.InventoryReservation
	for _, res := range r.reservations {
		if (input.ReservationID != "" && res.ID == input.ReservationID) ||
			(input.OrderID != "" && res.OrderID == input.OrderID) {
			matching = append(matching, res)
		}
	}

	if len(matching) == 0 {
		return appErrors.NotFound("reservation not found")
	}

	targetStatus := model.ReservationStatusReleased
	if strings.EqualFold(input.Reason, "EXPIRED") {
		targetStatus = model.ReservationStatusExpired
	}

	now := time.Now().UTC()
	for _, res := range matching {
		if res.Status == model.ReservationStatusReserved {
			for _, inv := range r.inventories {
				if inv.ProductID == res.ProductID {
					if (res.VariantID == nil && inv.VariantID == nil) ||
						(res.VariantID != nil && inv.VariantID != nil && *res.VariantID == *inv.VariantID) {
						inv.AvailableQuantity += res.Quantity
						inv.ReservedQuantity -= res.Quantity
						if inv.ReservedQuantity < 0 {
							inv.ReservedQuantity = 0
						}
						inv.UpdatedAt = now

						ref := res.OrderID
						notes := fmt.Sprintf("Stock released (%s)", input.Reason)
						r.transactions = append(r.transactions, &model.InventoryTransaction{
							ID:          uuid.NewString(),
							InventoryID: inv.ID,
							Type:        model.TransactionTypeRelease,
							Quantity:    res.Quantity,
							ReferenceID: &ref,
							Notes:       &notes,
							CreatedAt:   now,
						})
						break
					}
				}
			}
			res.Status = targetStatus
			res.UpdatedAt = now
		}
	}

	return nil
}

func (r *memoryInventoryRepo) CommitStock(ctx context.Context, input dto.CommitStockInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var matching []*model.InventoryReservation
	var confirmedCount int
	var releasedCount int

	for _, res := range r.reservations {
		if (input.ReservationID != "" && res.ID == input.ReservationID) ||
			(input.OrderID != "" && res.OrderID == input.OrderID) {
			matching = append(matching, res)
			if res.Status == model.ReservationStatusConfirmed {
				confirmedCount++
			} else if res.Status == model.ReservationStatusReleased || res.Status == model.ReservationStatusExpired {
				releasedCount++
			}
		}
	}

	if len(matching) == 0 {
		return appErrors.NotFound("reservation not found")
	}

	var active []*model.InventoryReservation
	for _, res := range matching {
		if res.Status == model.ReservationStatusReserved {
			active = append(active, res)
		}
	}

	if len(active) == 0 {
		if confirmedCount > 0 {
			return appErrors.Conflict("reservation is already committed")
		}
		if releasedCount > 0 {
			return appErrors.Conflict("cannot commit released or expired reservation")
		}
		return appErrors.Conflict("reservation is not in a committable state")
	}

	now := time.Now().UTC()
	for _, res := range active {
		for _, inv := range r.inventories {
			if inv.ProductID == res.ProductID {
				if (res.VariantID == nil && inv.VariantID == nil) ||
					(res.VariantID != nil && inv.VariantID != nil && *res.VariantID == *inv.VariantID) {
					inv.ReservedQuantity -= res.Quantity
					if inv.ReservedQuantity < 0 {
						inv.ReservedQuantity = 0
					}
					inv.UpdatedAt = now

					ref := res.OrderID
					notes := fmt.Sprintf("Stock committed for order %s", res.OrderID)
					r.transactions = append(r.transactions, &model.InventoryTransaction{
						ID:          uuid.NewString(),
						InventoryID: inv.ID,
						Type:        model.TransactionTypeCommit,
						Quantity:    res.Quantity,
						ReferenceID: &ref,
						Notes:       &notes,
						CreatedAt:   now,
					})
					break
				}
			}
		}
		res.Status = model.ReservationStatusConfirmed
		res.UpdatedAt = now
	}

	return nil
}

func (r *memoryInventoryRepo) ReleaseExpiredReservations(ctx context.Context) (int, error) {
	r.mu.Lock()
	now := time.Now().UTC()
	var expiredIDs []string
	for _, res := range r.reservations {
		if res.Status == model.ReservationStatusReserved && !res.ExpiresAt.After(now) {
			expiredIDs = append(expiredIDs, res.ID)
		}
	}
	r.mu.Unlock()

	released := 0
	for _, id := range expiredIDs {
		err := r.ReleaseStock(ctx, dto.ReleaseStockInput{
			ReservationID: id,
			Reason:        "EXPIRED",
		})
		if err == nil {
			released++
		}
	}
	return released, nil
}

func (r *memoryInventoryRepo) GetTransactionsByInventoryID(ctx context.Context, inventoryID string, limit, offset int) ([]*model.InventoryTransaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.InventoryTransaction
	for _, tx := range r.transactions {
		if tx.InventoryID == inventoryID {
			result = append(result, tx)
		}
	}
	return result, nil
}

type mockProductClient struct {
	products          map[string]*client.ProductDetails
	merchantOwnership map[string]string
}

func newMockProductClient() *mockProductClient {
	return &mockProductClient{
		products:          make(map[string]*client.ProductDetails),
		merchantOwnership: make(map[string]string),
	}
}

func (s *mockProductClient) AddProduct(id, storeID, sku, merchantID string) {
	s.products[id] = &client.ProductDetails{
		ID:       id,
		StoreID:  storeID,
		SKU:      sku,
		Name:     "Product " + id,
		IsActive: true,
	}
	if merchantID != "" {
		s.merchantOwnership[id] = merchantID
	}
}

func (s *mockProductClient) GetProduct(ctx context.Context, productID string) (*client.ProductDetails, error) {
	if prod, ok := s.products[productID]; ok {
		return prod, nil
	}
	if len(s.products) > 0 {
		return nil, appErrors.NotFound("product not found")
	}
	return &client.ProductDetails{
		ID:       productID,
		StoreID:  "test-store-id",
		SKU:      "SKU-DEFAULT",
		Name:     "Default Test Product",
		IsActive: true,
	}, nil
}

func (s *mockProductClient) ValidateProductVariant(ctx context.Context, productID string, variantID *string) error {
	prod, err := s.GetProduct(ctx, productID)
	if err != nil {
		return err
	}
	if prod == nil {
		return appErrors.NotFound("invalid product reference")
	}
	if variantID != nil && *variantID == "invalid-variant-id" {
		return appErrors.NotFound("invalid variant reference")
	}
	return nil
}

func (s *mockProductClient) VerifyProductMerchant(ctx context.Context, merchantID, productID string) (bool, error) {
	if merchantID == "" {
		return true, nil
	}
	if expectedMerchant, ok := s.merchantOwnership[productID]; ok {
		return expectedMerchant == merchantID, nil
	}
	return true, nil
}

func setupTestServer() (*inventoryGRPC.InventoryGRPCServer, *mockProductClient) {
	mockCli := newMockProductClient()
	repo := newMemoryInventoryRepo()
	svc := service.NewInventoryService(repo, mockCli)
	server := inventoryGRPC.NewInventoryGRPCServer(svc)
	return server, mockCli
}

func TestInventoryGRPCServer_CreateInventory_Success(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "SKU-TEST-1", merchantID)

	req := &inventorypb.CreateInventoryRequest{
		MerchantId:        merchantID,
		ProductId:         prodID,
		Sku:               "SKU-TEST-1",
		InitialQuantity:   100,
		LowStockThreshold: 10,
	}

	resp, err := server.CreateInventory(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	if resp.Inventory == nil {
		t.Fatalf("expected inventory item, got nil")
	}
	if resp.Inventory.ProductId != prodID {
		t.Errorf("expected product_id %s, got %s", prodID, resp.Inventory.ProductId)
	}
	if resp.Inventory.AvailableQuantity != 100 {
		t.Errorf("expected available quantity 100, got %d", resp.Inventory.AvailableQuantity)
	}
	if resp.Inventory.ReservedQuantity != 0 {
		t.Errorf("expected reserved quantity 0, got %d", resp.Inventory.ReservedQuantity)
	}

	// Verify protobuf marshaling
	data, err := proto.Marshal(resp)
	if err != nil {
		t.Fatalf("proto.Marshal failed: %v", err)
	}
	var unmarshaled inventorypb.CreateInventoryResponse
	if err := proto.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("proto.Unmarshal failed: %v", err)
	}
	if unmarshaled.Inventory.Sku != "SKU-TEST-1" {
		t.Errorf("expected sku SKU-TEST-1, got %s", unmarshaled.Inventory.Sku)
	}
}

func TestInventoryGRPCServer_CreateInventory_AutoSKU(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	varID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "", merchantID)

	// Omit SKU to test automatic SKU generation
	req := &inventorypb.CreateInventoryRequest{
		MerchantId:        merchantID,
		ProductId:         prodID,
		VariantId:         varID,
		InitialQuantity:   50,
		LowStockThreshold: 8,
	}

	resp, err := server.CreateInventory(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateInventory failed with auto SKU: %v", err)
	}

	if resp.Inventory == nil {
		t.Fatalf("expected inventory item, got nil")
	}
	if !strings.HasPrefix(resp.Inventory.Sku, "SKU-") {
		t.Errorf("expected auto-generated SKU with prefix 'SKU-', got %s", resp.Inventory.Sku)
	}
	if len(resp.Inventory.Sku) != 12 {
		t.Errorf("expected auto-generated SKU length 12 (e.g. SKU-XXXXXXXX), got %s (len %d)", resp.Inventory.Sku, len(resp.Inventory.Sku))
	}
	if resp.Inventory.LowStockThreshold != 8 {
		t.Errorf("expected low stock threshold 8, got %d", resp.Inventory.LowStockThreshold)
	}
}

func TestInventoryGRPCServer_CreateInventory_InvalidProduct(t *testing.T) {
	server, _ := setupTestServer()

	req := &inventorypb.CreateInventoryRequest{
		MerchantId:      "merch-123",
		ProductId:       "not-a-valid-uuid",
		Sku:             "SKU-TEST",
		InitialQuantity: 10,
	}

	_, err := server.CreateInventory(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error for invalid UUID product id, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument code, got %v", st.Code())
	}
}

func TestInventoryGRPCServer_CreateInventory_MerchantMismatch(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	stubCli.AddProduct(prodID, "store-1", "SKU-1", "merch-owner")

	req := &inventorypb.CreateInventoryRequest{
		MerchantId:      "merch-attacker",
		ProductId:       prodID,
		Sku:             "SKU-1",
		InitialQuantity: 50,
	}

	_, err := server.CreateInventory(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error when merchant doesn't own product, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}

func TestInventoryGRPCServer_RestockInventory_Success(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "SKU-RESTOCK", merchantID)

	// Create inventory first
	createResp, err := server.CreateInventory(context.Background(), &inventorypb.CreateInventoryRequest{
		MerchantId:      merchantID,
		ProductId:       prodID,
		Sku:             "SKU-RESTOCK",
		InitialQuantity: 20,
	})
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	// Restock
	notes := "Weekly restock"
	restockResp, err := server.RestockInventory(context.Background(), &inventorypb.RestockInventoryRequest{
		MerchantId:  merchantID,
		InventoryId: createResp.Inventory.InventoryId,
		Quantity:    30,
		Notes:       notes,
	})
	if err != nil {
		t.Fatalf("RestockInventory failed: %v", err)
	}

	if !restockResp.Success {
		t.Errorf("expected restock success true")
	}
	if restockResp.Inventory.AvailableQuantity != 50 {
		t.Errorf("expected available quantity 50, got %d", restockResp.Inventory.AvailableQuantity)
	}
	if restockResp.Inventory.ReservedQuantity != 0 {
		t.Errorf("expected reserved quantity 0, got %d", restockResp.Inventory.ReservedQuantity)
	}
}

func TestInventoryGRPCServer_RestockInventory_ZeroOrNegativeQuantity(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "SKU-INVALID-RESTOCK", merchantID)

	createResp, err := server.CreateInventory(context.Background(), &inventorypb.CreateInventoryRequest{
		MerchantId:      merchantID,
		ProductId:       prodID,
		Sku:             "SKU-INVALID-RESTOCK",
		InitialQuantity: 10,
	})
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	// Try restock 0
	_, err = server.RestockInventory(context.Background(), &inventorypb.RestockInventoryRequest{
		MerchantId:  merchantID,
		InventoryId: createResp.Inventory.InventoryId,
		Quantity:    0,
	})
	if err == nil {
		t.Fatalf("expected error for quantity 0, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}

	// Try restock negative
	_, err = server.RestockInventory(context.Background(), &inventorypb.RestockInventoryRequest{
		MerchantId:  merchantID,
		InventoryId: createResp.Inventory.InventoryId,
		Quantity:    -5,
	})
	if err == nil {
		t.Fatalf("expected error for negative quantity, got nil")
	}
}

func TestInventoryGRPCServer_RestockInventory_MerchantMismatch(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	stubCli.AddProduct(prodID, "store-1", "SKU-RESTOCK-MISMATCH", "merch-owner")

	createResp, err := server.CreateInventory(context.Background(), &inventorypb.CreateInventoryRequest{
		MerchantId:      "merch-owner",
		ProductId:       prodID,
		Sku:             "SKU-RESTOCK-MISMATCH",
		InitialQuantity: 10,
	})
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	_, err = server.RestockInventory(context.Background(), &inventorypb.RestockInventoryRequest{
		MerchantId:  "merch-unauthorized",
		InventoryId: createResp.Inventory.InventoryId,
		Quantity:    10,
	})
	if err == nil {
		t.Fatalf("expected PermissionDenied for unauthorized merchant, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}

func TestInventoryGRPCServer_GetInventory_ProductAndVariant(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	varID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "SKU-VAR", merchantID)

	// Create variant inventory
	createResp, err := server.CreateInventory(context.Background(), &inventorypb.CreateInventoryRequest{
		MerchantId:      merchantID,
		ProductId:       prodID,
		VariantId:       varID,
		Sku:             "SKU-VAR",
		InitialQuantity: 40,
	})
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	// Retrieve by product and variant
	getResp, err := server.GetInventory(context.Background(), &inventorypb.GetInventoryRequest{
		MerchantId: merchantID,
		ProductId:  prodID,
		VariantId:  varID,
	})
	if err != nil {
		t.Fatalf("GetInventory failed: %v", err)
	}
	if getResp.Inventory.InventoryId != createResp.Inventory.InventoryId {
		t.Errorf("expected inventory id %s, got %s", createResp.Inventory.InventoryId, getResp.Inventory.InventoryId)
	}
	if getResp.Inventory.VariantId != varID {
		t.Errorf("expected variant id %s, got %s", varID, getResp.Inventory.VariantId)
	}
}

func TestInventoryGRPCServer_ReserveAndReleaseStock_SeparateTracking(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "SKU-RESERVE", merchantID)

	_, err := server.CreateInventory(context.Background(), &inventorypb.CreateInventoryRequest{
		MerchantId:      merchantID,
		ProductId:       prodID,
		Sku:             "SKU-RESERVE",
		InitialQuantity: 50,
	})
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	orderID := uuid.NewString()
	reserveResp, err := server.ReserveStock(context.Background(), &inventorypb.ReserveStockRequest{
		OrderId: orderID,
		Items: []*inventorypb.ReservationItem{
			{
				ProductId: prodID,
				Quantity:  15,
			},
		},
	})
	if err != nil {
		t.Fatalf("ReserveStock failed: %v", err)
	}
	if !reserveResp.Success || reserveResp.ReservationId == "" {
		t.Fatalf("expected successful reservation id, got %+v", reserveResp)
	}

	// Check stock item shows available=35, reserved=15
	stockResp, err := server.GetStock(context.Background(), &inventorypb.GetStockRequest{ProductId: prodID})
	if err != nil {
		t.Fatalf("GetStock failed: %v", err)
	}
	if stockResp.Stock.AvailableQuantity != 35 {
		t.Errorf("expected available quantity 35, got %d", stockResp.Stock.AvailableQuantity)
	}
	if stockResp.Stock.ReservedQuantity != 15 {
		t.Errorf("expected reserved quantity 15, got %d", stockResp.Stock.ReservedQuantity)
	}

	// Release stock
	releaseResp, err := server.ReleaseStock(context.Background(), &inventorypb.ReleaseStockRequest{
		ReservationId: reserveResp.ReservationId,
	})
	if err != nil {
		t.Fatalf("ReleaseStock failed: %v", err)
	}
	if !releaseResp.Success {
		t.Errorf("expected release success true")
	}

	// After release, available should be back to 50, reserved to 0
	stockRespAfter, err := server.GetStock(context.Background(), &inventorypb.GetStockRequest{ProductId: prodID})
	if err != nil {
		t.Fatalf("GetStock failed: %v", err)
	}
	if stockRespAfter.Stock.AvailableQuantity != 50 {
		t.Errorf("expected available quantity 50 after release, got %d", stockRespAfter.Stock.AvailableQuantity)
	}
	if stockRespAfter.Stock.ReservedQuantity != 0 {
		t.Errorf("expected reserved quantity 0 after release, got %d", stockRespAfter.Stock.ReservedQuantity)
	}
}

func TestInventoryGRPCServer_ReserveStock_InsufficientStock(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "SKU-LIMITED", merchantID)

	_, err := server.CreateInventory(context.Background(), &inventorypb.CreateInventoryRequest{
		MerchantId:      merchantID,
		ProductId:       prodID,
		Sku:             "SKU-LIMITED",
		InitialQuantity: 10,
	})
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	orderID := uuid.NewString()
	_, err = server.ReserveStock(context.Background(), &inventorypb.ReserveStockRequest{
		OrderId: orderID,
		Items: []*inventorypb.ReservationItem{
			{
				ProductId: prodID,
				Quantity:  20,
			},
		},
	})
	if err == nil {
		t.Fatalf("expected error for insufficient stock reservation, got nil")
	}

	st, ok := status.FromError(err)
	if !ok || (st.Code() != codes.AlreadyExists && st.Code() != codes.InvalidArgument && st.Code() != codes.FailedPrecondition) {
		t.Fatalf("expected Conflict or InvalidArgument status code, got %v", err)
	}
}

func TestInventoryGRPCServer_ReserveAndRelease_VariantAndExpiration(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	varID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "SKU-VAR-RES", merchantID)

	_, err := server.CreateInventory(context.Background(), &inventorypb.CreateInventoryRequest{
		MerchantId:      merchantID,
		ProductId:       prodID,
		VariantId:       varID,
		Sku:             "SKU-VAR-RES",
		InitialQuantity: 30,
	})
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	orderID := uuid.NewString()
	reserveResp, err := server.ReserveStock(context.Background(), &inventorypb.ReserveStockRequest{
		OrderId: orderID,
		Items: []*inventorypb.ReservationItem{
			{
				ProductId: prodID,
				VariantId: varID,
				Quantity:  10,
			},
		},
		ExpirationMinutes: 15,
	})
	if err != nil {
		t.Fatalf("ReserveStock with variant failed: %v", err)
	}

	if reserveResp.ExpiresAt == "" {
		t.Errorf("expected expires_at in reservation response")
	}

	// Verify stock
	getInvResp, err := server.GetInventory(context.Background(), &inventorypb.GetInventoryRequest{
		MerchantId: merchantID,
		ProductId:  prodID,
		VariantId:  varID,
	})
	if err != nil {
		t.Fatalf("GetInventory failed: %v", err)
	}
	if getInvResp.Inventory.AvailableQuantity != 20 {
		t.Errorf("expected available quantity 20, got %d", getInvResp.Inventory.AvailableQuantity)
	}
	if getInvResp.Inventory.ReservedQuantity != 10 {
		t.Errorf("expected reserved quantity 10, got %d", getInvResp.Inventory.ReservedQuantity)
	}
}

func TestInventoryGRPCServer_ReleaseStock_DuplicateReleaseIdempotency(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "SKU-DUP", merchantID)

	_, err := server.CreateInventory(context.Background(), &inventorypb.CreateInventoryRequest{
		MerchantId:      merchantID,
		ProductId:       prodID,
		Sku:             "SKU-DUP",
		InitialQuantity: 40,
	})
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	orderID := uuid.NewString()
	resResp, err := server.ReserveStock(context.Background(), &inventorypb.ReserveStockRequest{
		OrderId: orderID,
		Items: []*inventorypb.ReservationItem{
			{ProductId: prodID, Quantity: 10},
		},
	})
	if err != nil {
		t.Fatalf("ReserveStock failed: %v", err)
	}

	// 1st release call
	rel1, err := server.ReleaseStock(context.Background(), &inventorypb.ReleaseStockRequest{
		ReservationId: resResp.ReservationId,
		Reason:        "PAYMENT_FAILED",
	})
	if err != nil || !rel1.Success {
		t.Fatalf("1st ReleaseStock failed: %v", err)
	}

	// 2nd release call (duplicate release - e.g. order cancellation following payment failure)
	rel2, err := server.ReleaseStock(context.Background(), &inventorypb.ReleaseStockRequest{
		ReservationId: resResp.ReservationId,
		Reason:        "ORDER_CANCELLED",
	})
	if err != nil || !rel2.Success {
		t.Fatalf("2nd duplicate ReleaseStock failed: %v", err)
	}

	// Verify inventory quantity is restored once to 40, not double-restored to 50
	stockResp, err := server.GetStock(context.Background(), &inventorypb.GetStockRequest{ProductId: prodID})
	if err != nil {
		t.Fatalf("GetStock failed: %v", err)
	}
	if stockResp.Stock.AvailableQuantity != 40 {
		t.Errorf("expected available quantity 40 after duplicate release, got %d", stockResp.Stock.AvailableQuantity)
	}
	if stockResp.Stock.ReservedQuantity != 0 {
		t.Errorf("expected reserved quantity 0 after duplicate release, got %d", stockResp.Stock.ReservedQuantity)
	}
}

func TestInventoryGRPCServer_ReleaseStock_ByOrderId(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "SKU-ORDER-REL", merchantID)

	_, err := server.CreateInventory(context.Background(), &inventorypb.CreateInventoryRequest{
		MerchantId:      merchantID,
		ProductId:       prodID,
		Sku:             "SKU-ORDER-REL",
		InitialQuantity: 25,
	})
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	orderID := uuid.NewString()
	_, err = server.ReserveStock(context.Background(), &inventorypb.ReserveStockRequest{
		OrderId: orderID,
		Items: []*inventorypb.ReservationItem{
			{ProductId: prodID, Quantity: 5},
		},
	})
	if err != nil {
		t.Fatalf("ReserveStock failed: %v", err)
	}

	// Release by order ID (e.g. order cancelled event listener)
	relResp, err := server.ReleaseStock(context.Background(), &inventorypb.ReleaseStockRequest{
		OrderId: orderID,
		Reason:  "ORDER_CANCELLED",
	})
	if err != nil || !relResp.Success {
		t.Fatalf("ReleaseStock by order ID failed: %v", err)
	}

	stockResp, err := server.GetStock(context.Background(), &inventorypb.GetStockRequest{ProductId: prodID})
	if err != nil {
		t.Fatalf("GetStock failed: %v", err)
	}
	if stockResp.Stock.AvailableQuantity != 25 {
		t.Errorf("expected available quantity 25, got %d", stockResp.Stock.AvailableQuantity)
	}
}

func TestInventoryGRPCServer_CommitStock_Success(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "SKU-COMMIT", merchantID)

	_, err := server.CreateInventory(context.Background(), &inventorypb.CreateInventoryRequest{
		MerchantId:      merchantID,
		ProductId:       prodID,
		Sku:             "SKU-COMMIT",
		InitialQuantity: 50,
	})
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	orderID := uuid.NewString()
	resResp, err := server.ReserveStock(context.Background(), &inventorypb.ReserveStockRequest{
		OrderId: orderID,
		Items: []*inventorypb.ReservationItem{
			{ProductId: prodID, Quantity: 15},
		},
	})
	if err != nil {
		t.Fatalf("ReserveStock failed: %v", err)
	}

	// Commit reservation
	commitResp, err := server.CommitStock(context.Background(), &inventorypb.CommitStockRequest{
		ReservationId: resResp.ReservationId,
	})
	if err != nil {
		t.Fatalf("CommitStock failed: %v", err)
	}
	if !commitResp.Success {
		t.Errorf("expected commit stock success true")
	}

	// Stock check: Available=35, Reserved=0 (permanently deducted)
	stockResp, err := server.GetStock(context.Background(), &inventorypb.GetStockRequest{ProductId: prodID})
	if err != nil {
		t.Fatalf("GetStock failed: %v", err)
	}
	if stockResp.Stock.AvailableQuantity != 35 {
		t.Errorf("expected available quantity 35, got %d", stockResp.Stock.AvailableQuantity)
	}
	if stockResp.Stock.ReservedQuantity != 0 {
		t.Errorf("expected reserved quantity 0 after commit, got %d", stockResp.Stock.ReservedQuantity)
	}
}

func TestInventoryGRPCServer_CommitStock_CannotCommitTwice(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "SKU-COMMIT-TWICE", merchantID)

	_, err := server.CreateInventory(context.Background(), &inventorypb.CreateInventoryRequest{
		MerchantId:      merchantID,
		ProductId:       prodID,
		Sku:             "SKU-COMMIT-TWICE",
		InitialQuantity: 20,
	})
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	orderID := uuid.NewString()
	resResp, err := server.ReserveStock(context.Background(), &inventorypb.ReserveStockRequest{
		OrderId: orderID,
		Items: []*inventorypb.ReservationItem{
			{ProductId: prodID, Quantity: 5},
		},
	})
	if err != nil {
		t.Fatalf("ReserveStock failed: %v", err)
	}

	// 1st commit succeeds
	_, err = server.CommitStock(context.Background(), &inventorypb.CommitStockRequest{
		ReservationId: resResp.ReservationId,
	})
	if err != nil {
		t.Fatalf("1st CommitStock failed: %v", err)
	}

	// 2nd commit must fail
	_, err = server.CommitStock(context.Background(), &inventorypb.CommitStockRequest{
		ReservationId: resResp.ReservationId,
	})
	if err == nil {
		t.Fatalf("expected error when committing reservation twice, got nil")
	}
}

func TestInventoryGRPCServer_CommitStock_CannotCommitReleased(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "SKU-COMMIT-RELEASED", merchantID)

	_, err := server.CreateInventory(context.Background(), &inventorypb.CreateInventoryRequest{
		MerchantId:      merchantID,
		ProductId:       prodID,
		Sku:             "SKU-COMMIT-RELEASED",
		InitialQuantity: 30,
	})
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	orderID := uuid.NewString()
	resResp, err := server.ReserveStock(context.Background(), &inventorypb.ReserveStockRequest{
		OrderId: orderID,
		Items: []*inventorypb.ReservationItem{
			{ProductId: prodID, Quantity: 10},
		},
	})
	if err != nil {
		t.Fatalf("ReserveStock failed: %v", err)
	}

	// Release stock
	_, err = server.ReleaseStock(context.Background(), &inventorypb.ReleaseStockRequest{
		ReservationId: resResp.ReservationId,
	})
	if err != nil {
		t.Fatalf("ReleaseStock failed: %v", err)
	}

	// Attempting commit on released reservation must fail
	_, err = server.CommitStock(context.Background(), &inventorypb.CommitStockRequest{
		ReservationId: resResp.ReservationId,
	})
	if err == nil {
		t.Fatalf("expected error committing released reservation, got nil")
	}
}

func TestInventoryGRPCServer_ConcurrentReservation_PreventsOverselling(t *testing.T) {
	server, stubCli := setupTestServer()
	prodID := uuid.NewString()
	merchantID := "merch-123"
	stubCli.AddProduct(prodID, "store-1", "SKU-CONCURRENT", merchantID)

	const totalStock = 20
	const numGoroutines = 50

	_, err := server.CreateInventory(context.Background(), &inventorypb.CreateInventoryRequest{
		MerchantId:      merchantID,
		ProductId:       prodID,
		Sku:             "SKU-CONCURRENT",
		InitialQuantity: totalStock,
	})
	if err != nil {
		t.Fatalf("CreateInventory failed: %v", err)
	}

	var wg sync.WaitGroup
	var successCount int32
	var failCount int32

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			orderID := uuid.NewString()
			_, err := server.ReserveStock(context.Background(), &inventorypb.ReserveStockRequest{
				OrderId: orderID,
				Items: []*inventorypb.ReservationItem{
					{ProductId: prodID, Quantity: 1},
				},
			})
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			} else {
				atomic.AddInt32(&failCount, 1)
			}
		}()
	}

	wg.Wait()

	if successCount != int32(totalStock) {
		t.Errorf("expected exactly %d successful reservations, got %d", totalStock, successCount)
	}
	if failCount != int32(numGoroutines-totalStock) {
		t.Errorf("expected %d failed reservations due to insufficient stock, got %d", numGoroutines-totalStock, failCount)
	}

	stockResp, err := server.GetStock(context.Background(), &inventorypb.GetStockRequest{ProductId: prodID})
	if err != nil {
		t.Fatalf("GetStock failed: %v", err)
	}
	if stockResp.Stock.AvailableQuantity != 0 {
		t.Errorf("expected available quantity 0, got %d", stockResp.Stock.AvailableQuantity)
	}
	if stockResp.Stock.ReservedQuantity != totalStock {
		t.Errorf("expected reserved quantity %d, got %d", totalStock, stockResp.Stock.ReservedQuantity)
	}
}
