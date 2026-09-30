package handler

import (
	"context"
	"log/slog"
	"time"

	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/cart-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CartGRPCHandler struct {
	cartpb.UnimplementedCartServiceServer
	cartService service.CartService
	log         *slog.Logger
}

func NewCartGRPCHandler(cartService service.CartService, log *slog.Logger) *CartGRPCHandler {
	return &CartGRPCHandler{
		cartService: cartService,
		log:         log,
	}
}

func (h *CartGRPCHandler) validateUserAuth(ctx context.Context, targetUserID string) (string, error) {
	authUserID := grpcclient.GetUserID(ctx)
	if authUserID != "" && targetUserID != "" && authUserID != targetUserID {
		return "", status.Error(codes.PermissionDenied, "user cannot access another user's cart")
	}
	if targetUserID != "" {
		return targetUserID, nil
	}
	if authUserID != "" {
		return authUserID, nil
	}
	return "", status.Error(codes.InvalidArgument, "user_id is required")
}

func (h *CartGRPCHandler) GetCart(ctx context.Context, req *cartpb.GetCartRequest) (*cartpb.GetCartResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request body cannot be nil")
	}
	userID, err := h.validateUserAuth(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	cart, err := h.cartService.GetCart(ctx, userID)
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &cartpb.GetCartResponse{
		Cart: cartToProto(cart),
	}, nil
}

func (h *CartGRPCHandler) AddCartItem(ctx context.Context, req *cartpb.AddCartItemRequest) (*cartpb.AddCartItemResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request body cannot be nil")
	}
	userID, err := h.validateUserAuth(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	cart, err := h.cartService.AddCartItem(ctx, dto.AddCartItemRequest{
		UserID:    userID,
		ProductID: req.GetProductId(),
		VariantID: req.GetVariantId(),
		StoreID:   req.GetStoreId(),
		UnitPrice: req.GetUnitPrice(),
		Quantity:  req.GetQuantity(),
	})
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &cartpb.AddCartItemResponse{
		Cart: cartToProto(cart),
	}, nil
}

func (h *CartGRPCHandler) UpdateCartItem(ctx context.Context, req *cartpb.UpdateCartItemRequest) (*cartpb.UpdateCartItemResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request body cannot be nil")
	}
	userID, err := h.validateUserAuth(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	cart, err := h.cartService.UpdateCartItem(ctx, dto.UpdateCartItemRequest{
		UserID:    userID,
		ProductID: req.GetProductId(),
		VariantID: req.GetVariantId(),
		Quantity:  req.GetQuantity(),
	})
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &cartpb.UpdateCartItemResponse{
		Cart: cartToProto(cart),
	}, nil
}

func (h *CartGRPCHandler) RemoveCartItem(ctx context.Context, req *cartpb.RemoveCartItemRequest) (*cartpb.RemoveCartItemResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request body cannot be nil")
	}
	userID, err := h.validateUserAuth(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	cart, err := h.cartService.RemoveCartItem(ctx, dto.RemoveCartItemRequest{
		UserID:    userID,
		ProductID: req.GetProductId(),
		VariantID: req.GetVariantId(),
	})
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &cartpb.RemoveCartItemResponse{
		Cart: cartToProto(cart),
	}, nil
}

func (h *CartGRPCHandler) ClearCart(ctx context.Context, req *cartpb.ClearCartRequest) (*cartpb.ClearCartResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request body cannot be nil")
	}
	userID, err := h.validateUserAuth(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	cart, err := h.cartService.ClearCart(ctx, userID)
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &cartpb.ClearCartResponse{
		Success: true,
		Cart:    cartToProto(cart),
	}, nil
}

func cartToProto(c *model.Cart) *cartpb.Cart {
	if c == nil {
		return nil
	}
	pbItems := make([]*cartpb.CartItem, 0, len(c.Items))
	for _, item := range c.Items {
		pbItems = append(pbItems, &cartpb.CartItem{
			Id:        item.ID,
			ProductId: item.ProductID,
			VariantId: item.VariantID,
			StoreId:   item.StoreID,
			UnitPrice: item.UnitPrice,
			Quantity:  item.Quantity,
		})
	}
	return &cartpb.Cart{
		Id:          c.ID,
		UserId:      c.UserID,
		Items:       pbItems,
		TotalAmount: c.TotalAmount,
		UpdatedAt:   c.UpdatedAt.Format(time.RFC3339),
	}
}
