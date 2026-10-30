package handler

import (
	"context"
	"log/slog"
	"time"

	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type OrderGRPCHandler struct {
	orderpb.UnimplementedOrderServiceServer
	svc service.OrderService
	log *slog.Logger
}

func NewOrderGRPCHandler(svc service.OrderService, log *slog.Logger) *OrderGRPCHandler {
	if log == nil {
		log = slog.Default()
	}
	return &OrderGRPCHandler{
		svc: svc,
		log: log,
	}
}

func (h *OrderGRPCHandler) CreateOrder(ctx context.Context, req *orderpb.CreateOrderRequest) (*orderpb.CreateOrderResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if cids := md.Get("x-correlation-id"); len(cids) > 0 && cids[0] != "" {
			ctx = logger.WithCorrelationID(ctx, cids[0])
		}
		if rids := md.Get("x-request-id"); len(rids) > 0 && rids[0] != "" {
			ctx = logger.WithRequestID(ctx, rids[0])
		}
	}

	result, err := h.svc.CreateOrder(ctx, dto.CreateOrderInput{
		UserID:            req.GetUserId(),
		CartID:            req.GetCartId(),
		ShippingAddress:   req.GetShippingAddress(),
		ShippingAddressID: req.GetShippingAddressId(),
		BillingAddressID:  req.GetBillingAddressId(),
		Currency:          req.GetCurrency(),
		IdempotencyKey:    req.GetIdempotencyKey(),
		ShippingFee:       req.GetShippingFee(),
		DiscountAmount:    req.GetDiscountAmount(),
	})
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	protoOrders := make([]*orderpb.Order, len(result.Orders))
	for i, o := range result.Orders {
		protoOrders[i] = mapModelToProtoOrder(o)
	}

	return &orderpb.CreateOrderResponse{
		Order:         mapModelToProtoOrder(result.PrimaryOrder),
		Orders:        protoOrders,
		ParentOrderId: result.ParentOrderID,
	}, nil
}

func (h *OrderGRPCHandler) GetOrder(ctx context.Context, req *orderpb.GetOrderRequest) (*orderpb.GetOrderResponse, error) {
	if req == nil || req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order id is required")
	}

	ord, err := h.svc.GetOrder(ctx, req.GetId(), req.GetUserId())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &orderpb.GetOrderResponse{
		Order: mapModelToProtoOrder(ord),
	}, nil
}

func (h *OrderGRPCHandler) ListOrders(ctx context.Context, req *orderpb.ListOrdersRequest) (*orderpb.ListOrdersResponse, error) {
	if req == nil || req.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	result, err := h.svc.ListOrders(ctx, dto.ListOrdersInput{
		UserID: req.GetUserId(),
		Limit:  int(req.GetLimit()),
		Offset: int(req.GetOffset()),
	})
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	protoOrders := make([]*orderpb.Order, len(result.Orders))
	for i, o := range result.Orders {
		protoOrders[i] = mapModelToProtoOrder(o)
	}

	return &orderpb.ListOrdersResponse{
		Orders: protoOrders,
		Total:  int32(result.Total),
	}, nil
}

func (h *OrderGRPCHandler) UpdateOrderStatus(ctx context.Context, req *orderpb.UpdateOrderStatusRequest) (*orderpb.UpdateOrderStatusResponse, error) {
	if req == nil || req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order id is required")
	}

	ord, err := h.svc.UpdateOrderStatus(ctx, req.GetId(), model.OrderStatus(req.GetStatus()))
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &orderpb.UpdateOrderStatusResponse{
		Order: mapModelToProtoOrder(ord),
	}, nil
}

func (h *OrderGRPCHandler) CancelOrder(ctx context.Context, req *orderpb.CancelOrderRequest) (*orderpb.CancelOrderResponse, error) {
	if req == nil || req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order id is required")
	}

	ord, err := h.svc.CancelOrder(ctx, req.GetId(), req.GetUserId(), req.GetReason())
	if err != nil {
		return nil, appErrors.ToGRPC(err)
	}

	return &orderpb.CancelOrderResponse{
		Order: mapModelToProtoOrder(ord),
	}, nil
}

func mapModelToProtoOrder(o *model.Order) *orderpb.Order {
	if o == nil {
		return nil
	}

	protoItems := make([]*orderpb.OrderItem, len(o.Items))
	for i, it := range o.Items {
		protoItems[i] = &orderpb.OrderItem{
			Id:          it.ID,
			ProductId:   it.ProductID,
			StoreId:     it.StoreID,
			Sku:         it.SKU,
			ProductName: it.ProductName,
			Quantity:    int32(it.Quantity),
			UnitPrice:   it.UnitPrice,
			Tax:         it.Tax,
			Discount:    it.Discount,
			TotalPrice:  it.TotalPrice,
		}
	}

	parentID := ""
	if o.ParentOrderID != nil {
		parentID = *o.ParentOrderID
	}
	shipAddrID := ""
	if o.ShippingAddressID != nil {
		shipAddrID = *o.ShippingAddressID
	}
	billAddrID := ""
	if o.BillingAddressID != nil {
		billAddrID = *o.BillingAddressID
	}

	return &orderpb.Order{
		Id:                o.ID,
		OrderNumber:       o.OrderNumber,
		ParentOrderId:     parentID,
		UserId:            o.UserID,
		StoreId:           o.StoreID,
		Status:            string(o.Status),
		OrderStatus:       string(o.Status),
		PaymentStatus:     string(o.PaymentStatus),
		Subtotal:          o.Subtotal,
		TaxAmount:         o.TaxAmount,
		ShippingFee:       o.ShippingFee,
		DiscountAmount:    o.DiscountAmount,
		TotalAmount:       o.TotalAmount,
		Currency:          o.Currency,
		ShippingAddress:   o.ShippingAddress,
		ShippingAddressId: shipAddrID,
		BillingAddressId:  billAddrID,
		Items:             protoItems,
		CreatedAt:         o.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         o.UpdatedAt.Format(time.RFC3339),
	}
}
