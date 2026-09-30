package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/repository"
)

type CartService interface {
	GetCart(ctx context.Context, userID string) (*model.Cart, error)
	AddCartItem(ctx context.Context, req dto.AddCartItemRequest) (*model.Cart, error)
	UpdateCartItem(ctx context.Context, req dto.UpdateCartItemRequest) (*model.Cart, error)
	RemoveCartItem(ctx context.Context, req dto.RemoveCartItemRequest) (*model.Cart, error)
	ClearCart(ctx context.Context, userID string) (*model.Cart, error)
}

type cartService struct {
	repo repository.CartRepository
	ttl  time.Duration
	log  *slog.Logger
}

func NewCartService(repo repository.CartRepository, ttlSeconds int, log *slog.Logger) CartService {
	if ttlSeconds <= 0 {
		ttlSeconds = 604800
	}
	return &cartService{
		repo: repo,
		ttl:  time.Duration(ttlSeconds) * time.Second,
		log:  log,
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

	_ = s.repo.SetCartInCache(ctx, updatedCart, s.ttl)
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
			_ = s.repo.SetCartInCache(ctx, emptyCart, s.ttl)
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

	_ = s.repo.SetCartInCache(ctx, cart, s.ttl)
	return cart, nil
}
