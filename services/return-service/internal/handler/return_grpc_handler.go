package handler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	returns "github.com/marees-godev/GoCart-Server/contracts/protobuf/return"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ReturnGRPCHandler struct {
	returns.UnimplementedReturnServiceServer
	svc service.ReturnService
	log *slog.Logger
}

func NewReturnGRPCHandler(svc service.ReturnService, log *slog.Logger) *ReturnGRPCHandler {
	if log == nil {
		log = slog.Default()
	}
	return &ReturnGRPCHandler{
		svc: svc,
		log: log,
	}
}

func (h *ReturnGRPCHandler) RequestReturn(ctx context.Context, req *returns.RequestReturnRequest) (*returns.RequestReturnResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	ret, err := h.svc.RequestReturn(ctx, &dto.RequestReturnDTO{
		OrderID:     req.GetOrderId(),
		OrderItemID: req.GetOrderItemId(),
		UserID:      req.GetUserId(),
		Reason:      req.GetReason(),
		ItemIDs:     req.GetItemIds(),
		Type:        req.GetType(),
	})
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.RequestReturnResponse{
		ReturnOrder: mapModelToProtoReturn(ret),
	}, nil
}

func (h *ReturnGRPCHandler) GetReturn(ctx context.Context, req *returns.GetReturnRequest) (*returns.GetReturnResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	ret, err := h.svc.GetReturn(ctx, req.GetId())
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.GetReturnResponse{
		ReturnOrder: mapModelToProtoReturn(ret),
	}, nil
}

func (h *ReturnGRPCHandler) UpdateReturnStatus(ctx context.Context, req *returns.UpdateReturnStatusRequest) (*returns.UpdateReturnStatusResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	ret, err := h.svc.UpdateReturnStatus(ctx, req.GetId(), model.ReturnStatus(req.GetStatus()))
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.UpdateReturnStatusResponse{
		ReturnOrder: mapModelToProtoReturn(ret),
	}, nil
}

func (h *ReturnGRPCHandler) VerifyProduct(ctx context.Context, req *returns.VerifyProductRequest) (*returns.VerifyProductResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	ret, verification, err := h.svc.VerifyProduct(ctx, &dto.VerifyProductDTO{
		ReturnID:    req.GetReturnId(),
		VerifiedBy:  req.GetVerifiedBy(),
		Condition:   req.GetCondition(),
		IsApproved:  req.GetIsApproved(),
		ProofImages: req.GetProofImages(),
		Notes:       req.GetNotes(),
	})
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.VerifyProductResponse{
		ReturnOrder:  mapModelToProtoReturn(ret),
		Verification: mapVerificationToProto(verification),
	}, nil
}

func (h *ReturnGRPCHandler) ListReturns(ctx context.Context, req *returns.ListReturnsRequest) (*returns.ListReturnsResponse, error) {
	if req == nil {
		req = &returns.ListReturnsRequest{Limit: 20, Offset: 0}
	}

	returnsList, total, err := h.svc.ListReturns(ctx, &dto.ListReturnsDTO{
		Limit:  int(req.GetLimit()),
		Offset: int(req.GetOffset()),
		Status: req.GetStatus(),
		UserID: req.GetUserId(),
		Type:   req.GetType(),
	})
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	protoReturns := make([]*returns.ReturnOrder, len(returnsList))
	for i, r := range returnsList {
		protoReturns[i] = mapModelToProtoReturn(r)
	}

	return &returns.ListReturnsResponse{
		Returns: protoReturns,
		Total:   int32(total),
	}, nil
}

func (h *ReturnGRPCHandler) GetReturnsByOrder(ctx context.Context, req *returns.GetReturnsByOrderRequest) (*returns.GetReturnsByOrderResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	returnsList, err := h.svc.ListReturnsByOrderID(ctx, req.GetOrderId())
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	protoReturns := make([]*returns.ReturnOrder, len(returnsList))
	for i, r := range returnsList {
		protoReturns[i] = mapModelToProtoReturn(r)
	}

	return &returns.GetReturnsByOrderResponse{
		Returns: protoReturns,
	}, nil
}

func mapModelToProtoReturn(m *model.Return) *returns.ReturnOrder {
	if m == nil {
		return nil
	}
	ret := &returns.ReturnOrder{
		Id:           m.ID,
		OrderId:      m.OrderID,
		OrderItemId:  m.OrderItemID,
		UserId:       m.UserID,
		Reason:       m.Reason,
		Status:       string(m.Status),
		Type:         string(m.Type),
		RefundAmount: m.RefundAmount,
		RequestedAt:  m.RequestedAt.Format(time.RFC3339),
		CreatedAt:    m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    m.UpdatedAt.Format(time.RFC3339),
	}
	if m.ApprovedAt != nil {
		ret.ApprovedAt = m.ApprovedAt.Format(time.RFC3339)
	}
	if m.CompletedAt != nil {
		ret.CompletedAt = m.CompletedAt.Format(time.RFC3339)
	}
	if m.Verification != nil {
		ret.Verification = mapVerificationToProto(m.Verification)
	}
	return ret
}

func mapVerificationToProto(v *model.ProductVerification) *returns.ProductVerification {
	if v == nil {
		return nil
	}

	return &returns.ProductVerification{
		Id:          v.ID,
		ReturnId:    v.ReturnID,
		VerifiedBy:  v.VerifiedBy,
		Condition:   string(v.Condition),
		IsApproved:  v.IsApproved,
		ProofImages: v.ProofImages,
		Notes:       v.Notes,
		VerifiedAt:  v.VerifiedAt.Format(time.RFC3339),
		CreatedAt:   v.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   v.UpdatedAt.Format(time.RFC3339),
	}
}

func mapErrorToGRPC(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	switch {
	case errors.Is(err, service.ErrReturnReasonRequired),
		errors.Is(err, service.ErrOrderItemRequired),
		errors.Is(err, service.ErrOrderIDRequired),
		errors.Is(err, service.ErrUserIDRequired),
		errors.Is(err, service.ErrInvalidReturnStatus),
		errors.Is(err, service.ErrInvalidReturnType),
		errors.Is(err, service.ErrMultipleItemsNotSupported),
		errors.Is(err, service.ErrOrderItemMismatch),
		errors.Is(err, service.ErrProofImagesRequired),
		errors.Is(err, service.ErrVerifiedByRequired),
		errors.Is(err, service.ErrInvalidVerificationCondition):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrDuplicateReturnRequest):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, service.ErrOrderNotDelivered),
		errors.Is(err, service.ErrReturnWindowExpired),
		errors.Is(err, service.ErrProductNotReturnable),
		errors.Is(err, service.ErrReturnNotVerifiable),
		errors.Is(err, service.ErrInvalidStateTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, service.ErrOrderItemNotFound),
		errors.Is(err, service.ErrOrderNotFound),
		errors.Is(err, service.ErrReturnNotFound),
		errors.Is(err, repository.ErrReturnNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, service.ErrUnauthorizedReturnRequest):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

