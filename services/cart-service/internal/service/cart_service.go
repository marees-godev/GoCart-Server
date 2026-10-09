package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/repository"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CartService interface {
	GetCart(ctx context.Context, userID string) (*model.Cart, error)
	AddCartItem(ctx context.Context, req dto.AddCartItemRequest) (*model.Cart, error)
	UpdateCartItem(ctx context.Context, req dto.UpdateCartItemRequest) (*model.Cart, error)
	RemoveCartItem(ctx context.Context, req dto.RemoveCartItemRequest) (*model.Cart, error)
	ClearCart(ctx context.Context, userID string) (*model.Cart, error)
	ValidateCart(ctx context.Context, userID string) (*dto.ValidateCartResponse, error)
	PrepareCheckout(ctx context.Context, req dto.PrepareCheckoutRequest) (*dto.PrepareCheckoutResponse, error)
}

type cartService struct {
	repo            repository.CartRepository
	productClient   client.ProductClient
	inventoryClient client.InventoryClient
	clientTimeout   time.Duration
	ttl             time.Duration
	log             *slog.Logger
}

func NewCartService(repo repository.CartRepository, ttlSeconds int, log *slog.Logger) CartService {
	return NewCartServiceWithClients(repo, nil, nil, 5, ttlSeconds, log)
}

func NewCartServiceWithClients(
	repo repository.CartRepository,
	productClient client.ProductClient,
	inventoryClient client.InventoryClient,
	timeoutSeconds int,
	ttlSeconds int,
	log *slog.Logger,
) CartService {
	if ttlSeconds <= 0 {
		ttlSeconds = 604800
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}
	return &cartService{
		repo:            repo,
		productClient:   productClient,
		inventoryClient: inventoryClient,
		clientTimeout:   time.Duration(timeoutSeconds) * time.Second,
		ttl:             time.Duration(ttlSeconds) * time.Second,
		log:             log,
	}
}

func (s *cartService) GetCart(ctx context.Context, userID string) (*model.Cart, error) {
	if userID == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}

	cached, err := s.repo.GetCartFromCache(ctx, userID)
	if err == nil && cached != nil {
		return cached, nil
	}

	cart, err := s.repo.GetCartByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrCartNotFound) {
			created, createErr := s.repo.CreateCart(ctx, userID)
			if createErr != nil {
				return nil, appErrors.Internal(createErr, "failed to create cart for user")
			}
			cart = created
		} else {
			return nil, appErrors.Internal(err, "failed to get cart")
		}
	}

	_ = s.repo.SetCartInCache(ctx, cart, s.ttl)
	return cart, nil
}

func (s *cartService) AddCartItem(ctx context.Context, req dto.AddCartItemRequest) (*model.Cart, error) {
	if req.UserID == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}
	if req.ProductID == "" {
		return nil, appErrors.BadRequest("product_id is required")
	}
	if req.Quantity <= 0 {
		return nil, appErrors.BadRequest("quantity must be greater than zero")
	}

	cart, err := s.repo.GetCartByUserID(ctx, req.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrCartNotFound) {
			created, createErr := s.repo.CreateCart(ctx, req.UserID)
			if createErr != nil {
				return nil, appErrors.Internal(createErr, "failed to create cart")
			}
			cart = created
		} else {
			return nil, appErrors.Internal(err, "failed to retrieve cart")
		}
	}

	item := &model.CartItem{
		CartID:    cart.ID,
		ProductID: req.ProductID,
		VariantID: req.VariantID,
		StoreID:   req.StoreID,
		Quantity:  req.Quantity,
		UnitPrice: req.UnitPrice,
	}

	updatedCart, err := s.repo.AddOrUpdateItem(ctx, cart.ID, item)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to add item to cart")
	}

	_ = s.repo.SetCartInCache(ctx, updatedCart, s.ttl)
	return updatedCart, nil
}

func (s *cartService) UpdateCartItem(ctx context.Context, req dto.UpdateCartItemRequest) (*model.Cart, error) {
	if req.UserID == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}
	if req.ProductID == "" {
		return nil, appErrors.BadRequest("product_id is required")
	}
	if req.Quantity <= 0 {
		return nil, appErrors.BadRequest("quantity must be greater than zero")
	}

	cart, err := s.repo.GetCartByUserID(ctx, req.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrCartNotFound) {
			return nil, appErrors.NotFound("cart not found for user")
		}
		return nil, appErrors.Internal(err, "failed to retrieve cart")
	}

	updatedCart, err := s.repo.UpdateItemQuantity(ctx, cart.ID, req.ProductID, req.VariantID, req.Quantity)
	if err != nil {
		if errors.Is(err, repository.ErrItemNotFound) {
			return nil, appErrors.NotFound("item not found in cart")
		}
		return nil, appErrors.Internal(err, "failed to update item quantity")
	}

	_ = s.repo.SetCartInCache(ctx, updatedCart, s.ttl)
	return updatedCart, nil
}

func (s *cartService) RemoveCartItem(ctx context.Context, req dto.RemoveCartItemRequest) (*model.Cart, error) {
	if req.UserID == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}
	if req.ProductID == "" {
		return nil, appErrors.BadRequest("product_id is required")
	}

	cart, err := s.repo.GetCartByUserID(ctx, req.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrCartNotFound) {
			return nil, appErrors.NotFound("cart not found for user")
		}
		return nil, appErrors.Internal(err, "failed to retrieve cart")
	}

	updatedCart, err := s.repo.RemoveItem(ctx, cart.ID, req.ProductID, req.VariantID)
	if err != nil {
		if errors.Is(err, repository.ErrItemNotFound) {
			return nil, appErrors.NotFound("item not found in cart")
		}
		return nil, appErrors.Internal(err, "failed to remove item from cart")
	}

	if len(updatedCart.Items) == 0 {
		_ = s.repo.DeleteCartFromCache(ctx, req.UserID)
	} else {
		_ = s.repo.SetCartInCache(ctx, updatedCart, s.ttl)
	}

	return updatedCart, nil
}

func (s *cartService) ClearCart(ctx context.Context, userID string) (*model.Cart, error) {
	if userID == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}

	cart, err := s.repo.GetCartByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrCartNotFound) {
			emptyCart := &model.Cart{
				UserID: userID,
				Items:  make([]model.CartItem, 0),
			}
			_ = s.repo.DeleteCartFromCache(ctx, userID)
			return emptyCart, nil
		}
		return nil, appErrors.Internal(err, "failed to retrieve cart")
	}

	if err := s.repo.ClearCart(ctx, cart.ID); err != nil {
		return nil, appErrors.Internal(err, "failed to clear cart items")
	}

	cart.Items = make([]model.CartItem, 0)
	cart.CalculateTotal()
	cart.UpdatedAt = time.Now()

	_ = s.repo.DeleteCartFromCache(ctx, userID)
	return cart, nil
}

func (s *cartService) ValidateCart(ctx context.Context, userID string) (*dto.ValidateCartResponse, error) {
	if userID == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}

	cart, err := s.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(cart.Items) == 0 {
		return &dto.ValidateCartResponse{
			IsValid:     false,
			Cart:        cart,
			Errors:      []dto.ValidationError{{Message: "cart is empty", Code: dto.ErrCodeEmptyCart}},
			StoreGroups: []dto.StoreOrderGroup{},
		}, nil
	}

	isValid := true
	validationErrors := make([]dto.ValidationError, 0)
	storeMap := make(map[string][]model.CartItem)

	for i := range cart.Items {
		item := &cart.Items[i]

		if s.productClient != nil {
			callCtx, cancel := context.WithTimeout(ctx, s.clientTimeout)
			product, prodErr := s.productClient.GetProduct(callCtx, item.ProductID)
			cancel()

			if prodErr != nil {
				st, ok := status.FromError(prodErr)
				if ok && st.Code() == codes.NotFound {
					isValid = false
					validationErrors = append(validationErrors, dto.ValidationError{
						ProductID: item.ProductID,
						Message:   "product not found",
						Code:      dto.ErrCodeProductNotFound,
					})
				} else {
					return nil, appErrors.Internal(prodErr, fmt.Sprintf("failed to fetch product %s from product service", item.ProductID))
				}
			} else if product == nil {
				isValid = false
				validationErrors = append(validationErrors, dto.ValidationError{
					ProductID: item.ProductID,
					Message:   "product not found",
					Code:      dto.ErrCodeProductNotFound,
				})
			} else if product.GetStatus() == "inactive" || product.GetStatus() == "discontinued" {
				isValid = false
				validationErrors = append(validationErrors, dto.ValidationError{
					ProductID: item.ProductID,
					Message:   "product is inactive",
					Code:      dto.ErrCodeProductInactive,
				})
			} else {
				item.UnitPrice = product.GetPrice()
				if item.StoreID == "" && product.GetStoreId() != "" {
					item.StoreID = product.GetStoreId()
				}
			}
		}

		if s.inventoryClient != nil {
			callCtx, cancel := context.WithTimeout(ctx, s.clientTimeout)
			stock, invErr := s.inventoryClient.GetStock(callCtx, item.ProductID)
			cancel()

			if invErr != nil {
				st, ok := status.FromError(invErr)
				if ok && st.Code() == codes.NotFound {
					isValid = false
					validationErrors = append(validationErrors, dto.ValidationError{
						ProductID: item.ProductID,
						Message:   "out of stock",
						Code:      dto.ErrCodeOutOfStock,
					})
				} else {
					return nil, appErrors.Internal(invErr, fmt.Sprintf("failed to fetch stock for product %s from inventory service", item.ProductID))
				}
			} else if stock == nil || stock.GetAvailableQuantity() < item.Quantity {
				isValid = false
				avail := int32(0)
				if stock != nil {
					avail = stock.GetAvailableQuantity()
				}
				validationErrors = append(validationErrors, dto.ValidationError{
					ProductID: item.ProductID,
					Message:   fmt.Sprintf("insufficient stock: available %d, requested %d", avail, item.Quantity),
					Code:      dto.ErrCodeOutOfStock,
				})
			}
		}

		storeID := item.StoreID
		if storeID == "" {
			storeID = "default"
		}
		storeMap[storeID] = append(storeMap[storeID], *item)
	}

	cart.CalculateTotal()

	storeGroups := make([]dto.StoreOrderGroup, 0, len(storeMap))
	for storeID, items := range storeMap {
		var subtotal float64
		for _, it := range items {
			subtotal += float64(it.Quantity) * it.UnitPrice
		}
		storeGroups = append(storeGroups, dto.StoreOrderGroup{
			StoreID:  storeID,
			Items:    items,
			Subtotal: subtotal,
		})
	}

	return &dto.ValidateCartResponse{
		IsValid:     isValid,
		Cart:        cart,
		Errors:      validationErrors,
		StoreGroups: storeGroups,
	}, nil
}

func (s *cartService) PrepareCheckout(ctx context.Context, req dto.PrepareCheckoutRequest) (*dto.PrepareCheckoutResponse, error) {
	if req.UserID == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}

	valResp, err := s.ValidateCart(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	if !valResp.IsValid || len(valResp.Cart.Items) == 0 {
		return &dto.PrepareCheckoutResponse{
			IsValid: false,
			Errors:  valResp.Errors,
		}, nil
	}

	parentOrderID := uuid.New().String()
	orders := make([]dto.StoreOrderPayload, 0, len(valResp.StoreGroups))

	for _, group := range valResp.StoreGroups {
		orderPayload := dto.StoreOrderPayload{
			ParentOrderID:   parentOrderID,
			StoreID:         group.StoreID,
			UserID:          req.UserID,
			Items:           group.Items,
			Subtotal:        group.Subtotal,
			TotalAmount:     group.Subtotal,
			ShippingAddress: req.ShippingAddress,
		}
		orders = append(orders, orderPayload)
	}

	return &dto.PrepareCheckoutResponse{
		IsValid:       true,
		ParentOrderID: parentOrderID,
		Orders:        orders,
		Errors:        nil,
	}, nil
}
