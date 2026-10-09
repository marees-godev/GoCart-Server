package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/utils"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/repository"
)

type InventoryService interface {
	CreateInventory(ctx context.Context, input dto.CreateInventoryInput) (*model.Inventory, error)
	GetInventory(ctx context.Context, input dto.GetInventoryInput) (*model.Inventory, error)
	RestockInventory(ctx context.Context, input dto.RestockInventoryInput) (*model.Inventory, error)
	GetStock(ctx context.Context, productID string) (*model.Inventory, error)
	UpdateStock(ctx context.Context, input dto.UpdateStockInput) (*model.Inventory, error)
	ReserveStock(ctx context.Context, input dto.ReserveStockInput) (string, time.Time, error)
	ReleaseStock(ctx context.Context, input dto.ReleaseStockInput) error
	CommitStock(ctx context.Context, input dto.CommitStockInput) error
	ReleaseExpiredReservations(ctx context.Context) (int, error)
	StartExpirationWorker(ctx context.Context, interval time.Duration)
}

type inventoryService struct {
	repo          repository.InventoryRepository
	productClient client.ProductClient
}

func NewInventoryService(repo repository.InventoryRepository, productClient client.ProductClient) InventoryService {
	return &inventoryService{
		repo:          repo,
		productClient: productClient,
	}
}

func isValidUUID(u string) bool {
	_, err := uuid.Parse(strings.TrimSpace(u))
	return err == nil
}

func (s *inventoryService) CreateInventory(ctx context.Context, input dto.CreateInventoryInput) (*model.Inventory, error) {
	if !isValidUUID(input.ProductID) {
		return nil, appErrors.BadRequest("invalid product id: must be a valid UUID")
	}

	if input.VariantID != nil && strings.TrimSpace(*input.VariantID) != "" {
		if !isValidUUID(*input.VariantID) {
			return nil, appErrors.BadRequest("invalid variant id: must be a valid UUID")
		}
	}

	// Downstream verification: ensure product/variant reference exists
	var prodSKU string
	if s.productClient != nil {
		prod, err := s.productClient.GetProduct(ctx, input.ProductID)
		if err != nil {
			return nil, err
		}
		if prod != nil {
			prodSKU = strings.TrimSpace(prod.SKU)
		}

		if err := s.productClient.ValidateProductVariant(ctx, input.ProductID, input.VariantID); err != nil {
			return nil, err
		}

		if input.MerchantID != "" {
			owned, err := s.productClient.VerifyProductMerchant(ctx, input.MerchantID, input.ProductID)
			if err != nil {
				return nil, err
			}
			if !owned {
				return nil, appErrors.Forbidden("merchant cannot create inventory for a product belonging to another merchant")
			}
		}
	}

	sku := strings.TrimSpace(input.SKU)
	if sku == "" && prodSKU != "" {
		sku = prodSKU
	}
	if sku == "" {
		sku = utils.GenerateSKU()
	}

	var variantID *string
	if input.VariantID != nil && strings.TrimSpace(*input.VariantID) != "" {
		cleaned := strings.TrimSpace(*input.VariantID)
		variantID = &cleaned
	}

	threshold := input.LowStockThreshold
	if threshold == 0 {
		threshold = 5
	}

	inv := &model.Inventory{
		ProductID:         input.ProductID,
		VariantID:         variantID,
		SKU:               sku,
		AvailableQuantity: input.InitialQuantity,
		ReservedQuantity:  0,
		LowStockThreshold: threshold,
	}

	if err := s.repo.Create(ctx, inv, input.InitialQuantity, "Initial stock creation"); err != nil {
		return nil, err
	}

	return inv, nil
}

func (s *inventoryService) GetInventory(ctx context.Context, input dto.GetInventoryInput) (*model.Inventory, error) {
	var inv *model.Inventory
	var err error

	if input.InventoryID != nil && strings.TrimSpace(*input.InventoryID) != "" {
		if !isValidUUID(*input.InventoryID) {
			return nil, appErrors.BadRequest("invalid inventory id: must be a valid UUID")
		}
		inv, err = s.repo.GetByID(ctx, strings.TrimSpace(*input.InventoryID))
		if err != nil {
			return nil, err
		}
	} else if input.ProductID != nil && strings.TrimSpace(*input.ProductID) != "" {
		if !isValidUUID(*input.ProductID) {
			return nil, appErrors.BadRequest("invalid product id: must be a valid UUID")
		}

		var variantID *string
		if input.VariantID != nil && strings.TrimSpace(*input.VariantID) != "" {
			if !isValidUUID(*input.VariantID) {
				return nil, appErrors.BadRequest("invalid variant id: must be a valid UUID")
			}
			cleaned := strings.TrimSpace(*input.VariantID)
			variantID = &cleaned
		}

		inv, err = s.repo.GetByProductAndVariant(ctx, strings.TrimSpace(*input.ProductID), variantID)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, appErrors.BadRequest("either inventory_id or product_id must be provided")
	}

	// Verify merchant ownership if merchant_id is provided
	if input.MerchantID != nil && strings.TrimSpace(*input.MerchantID) != "" && s.productClient != nil {
		owned, err := s.productClient.VerifyProductMerchant(ctx, strings.TrimSpace(*input.MerchantID), inv.ProductID)
		if err != nil {
			return nil, err
		}
		if !owned {
			return nil, appErrors.Forbidden("merchant cannot view inventory belonging to another merchant")
		}
	}

	return inv, nil
}

func (s *inventoryService) RestockInventory(ctx context.Context, input dto.RestockInventoryInput) (*model.Inventory, error) {
	if input.Quantity <= 0 {
		return nil, appErrors.BadRequest("restock quantity must be greater than zero")
	}

	var existingInv *model.Inventory
	var err error

	if input.InventoryID != nil && strings.TrimSpace(*input.InventoryID) != "" {
		if !isValidUUID(*input.InventoryID) {
			return nil, appErrors.BadRequest("invalid inventory id: must be a valid UUID")
		}
		existingInv, err = s.repo.GetByID(ctx, strings.TrimSpace(*input.InventoryID))
		if err != nil {
			return nil, err
		}
	} else if input.ProductID != nil && strings.TrimSpace(*input.ProductID) != "" {
		if !isValidUUID(*input.ProductID) {
			return nil, appErrors.BadRequest("invalid product id: must be a valid UUID")
		}

		var variantID *string
		if input.VariantID != nil && strings.TrimSpace(*input.VariantID) != "" {
			if !isValidUUID(*input.VariantID) {
				return nil, appErrors.BadRequest("invalid variant id: must be a valid UUID")
			}
			cleaned := strings.TrimSpace(*input.VariantID)
			variantID = &cleaned
		}

		existingInv, err = s.repo.GetByProductAndVariant(ctx, strings.TrimSpace(*input.ProductID), variantID)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, appErrors.BadRequest("either inventory_id or product_id must be provided")
	}

	// Verify downstream merchant authorization
	if input.MerchantID != "" && s.productClient != nil {
		owned, err := s.productClient.VerifyProductMerchant(ctx, input.MerchantID, existingInv.ProductID)
		if err != nil {
			return nil, err
		}
		if !owned {
			return nil, appErrors.Forbidden("merchant cannot modify inventory belonging to another merchant")
		}
	}

	notes := "Merchant restocking"
	if input.Notes != nil && strings.TrimSpace(*input.Notes) != "" {
		notes = *input.Notes
	}

	return s.repo.Restock(ctx, existingInv.ID, input.Quantity, input.ReferenceID, &notes)
}

func (s *inventoryService) GetStock(ctx context.Context, productID string) (*model.Inventory, error) {
	if !isValidUUID(productID) {
		return nil, appErrors.BadRequest("invalid product id: must be a valid UUID")
	}
	return s.repo.GetByProductAndVariant(ctx, productID, nil)
}

func (s *inventoryService) UpdateStock(ctx context.Context, input dto.UpdateStockInput) (*model.Inventory, error) {
	if input.Quantity < 0 {
		return nil, appErrors.BadRequest("quantity cannot be negative")
	}

	var invID, prodID, varID *string

	if input.InventoryID != nil && strings.TrimSpace(*input.InventoryID) != "" {
		if !isValidUUID(*input.InventoryID) {
			return nil, appErrors.BadRequest("invalid inventory id: must be a valid UUID")
		}
		cleaned := strings.TrimSpace(*input.InventoryID)
		invID = &cleaned
	}

	if input.ProductID != nil && strings.TrimSpace(*input.ProductID) != "" {
		if !isValidUUID(*input.ProductID) {
			return nil, appErrors.BadRequest("invalid product id: must be a valid UUID")
		}
		cleaned := strings.TrimSpace(*input.ProductID)
		prodID = &cleaned
	}

	if input.VariantID != nil && strings.TrimSpace(*input.VariantID) != "" {
		if !isValidUUID(*input.VariantID) {
			return nil, appErrors.BadRequest("invalid variant id: must be a valid UUID")
		}
		cleaned := strings.TrimSpace(*input.VariantID)
		varID = &cleaned
	}

	if invID == nil && prodID == nil {
		return nil, appErrors.BadRequest("either inventory_id or product_id must be provided")
	}

	return s.repo.UpdateStock(ctx, dto.UpdateStockInput{
		InventoryID: invID,
		ProductID:   prodID,
		VariantID:   varID,
		Quantity:    input.Quantity,
	})
}

func (s *inventoryService) ReserveStock(ctx context.Context, input dto.ReserveStockInput) (string, time.Time, error) {
	if !isValidUUID(input.OrderID) {
		return "", time.Time{}, appErrors.BadRequest("invalid order id: must be a valid UUID")
	}
	if len(input.Items) == 0 {
		return "", time.Time{}, appErrors.BadRequest("no reservation items provided")
	}

	items := make([]model.ReserveItem, len(input.Items))
	for i, it := range input.Items {
		if !isValidUUID(it.ProductID) {
			return "", time.Time{}, appErrors.BadRequest("invalid product id in reservation items: must be a valid UUID")
		}
		if it.Quantity <= 0 {
			return "", time.Time{}, appErrors.BadRequest("reservation item quantity must be greater than zero")
		}
		var varID *string
		if it.VariantID != nil && strings.TrimSpace(*it.VariantID) != "" {
			if !isValidUUID(*it.VariantID) {
				return "", time.Time{}, appErrors.BadRequest("invalid variant id in reservation items: must be a valid UUID")
			}
			cleaned := strings.TrimSpace(*it.VariantID)
			varID = &cleaned
		}
		items[i] = model.ReserveItem{
			ProductID: it.ProductID,
			VariantID: varID,
			Quantity:  it.Quantity,
		}
	}

	expirationMinutes := input.ExpirationMinutes
	if expirationMinutes <= 0 {
		expirationMinutes = 15
	}
	expiresAt := time.Now().UTC().Add(time.Duration(expirationMinutes) * time.Minute)

	resID, err := s.repo.ReserveStock(ctx, input.OrderID, items, expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}

	return resID, expiresAt, nil
}

func (s *inventoryService) ReleaseStock(ctx context.Context, input dto.ReleaseStockInput) error {
	if strings.TrimSpace(input.ReservationID) != "" && !isValidUUID(input.ReservationID) {
		return appErrors.BadRequest("invalid reservation id: must be a valid UUID")
	}
	if strings.TrimSpace(input.OrderID) != "" && !isValidUUID(input.OrderID) {
		return appErrors.BadRequest("invalid order id: must be a valid UUID")
	}
	return s.repo.ReleaseStock(ctx, input)
}

func (s *inventoryService) CommitStock(ctx context.Context, input dto.CommitStockInput) error {
	if strings.TrimSpace(input.ReservationID) != "" && !isValidUUID(input.ReservationID) {
		return appErrors.BadRequest("invalid reservation id: must be a valid UUID")
	}
	if strings.TrimSpace(input.OrderID) != "" && !isValidUUID(input.OrderID) {
		return appErrors.BadRequest("invalid order id: must be a valid UUID")
	}
	return s.repo.CommitStock(ctx, input)
}

func (s *inventoryService) ReleaseExpiredReservations(ctx context.Context) (int, error) {
	return s.repo.ReleaseExpiredReservations(ctx)
}

func (s *inventoryService) StartExpirationWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if count, err := s.repo.ReleaseExpiredReservations(ctx); err != nil {
					logger.FromContext(ctx).Error("failed to release expired inventory reservations", "error", err)
				} else if count > 0 {
					logger.FromContext(ctx).Info("released expired inventory reservations", "count", count)
				}
			}
		}
	}()
}
