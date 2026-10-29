package handler

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	returns "github.com/marees-godev/GoCart-Server/contracts/protobuf/return"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
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

	orderItemID := strings.TrimSpace(req.GetOrderItemId())
	itemID := strings.TrimSpace(req.GetItemId())
	if orderItemID != "" && itemID != "" && orderItemID != itemID {
		return nil, status.Error(codes.InvalidArgument, "order_item_id and item_id mismatch")
	}
	if orderItemID == "" {
		orderItemID = itemID
	}
	if orderItemID == "" {
		return nil, status.Error(codes.InvalidArgument, "order_item_id is required")
	}

	ret, err := h.svc.RequestReturn(ctx, &dto.RequestReturnDTO{
		OrderID:     req.GetOrderId(),
		OrderItemID: orderItemID,
		ItemID:      orderItemID,
		UserID:      req.GetUserId(),
		Reason:      req.GetReason(),
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

	if req.GetReviewerId() != "" {
		ctx = auth.WithUser(ctx, &auth.UserContext{UserID: req.GetReviewerId()})
	}

	ret, err := h.svc.UpdateReturnStatus(ctx, req.GetId(), model.ReturnStatus(req.GetStatus()), req.GetRejectionReason())
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.UpdateReturnStatusResponse{
		ReturnOrder: mapModelToProtoReturn(ret),
	}, nil
}

func (h *ReturnGRPCHandler) ApproveReturn(ctx context.Context, req *returns.ApproveReturnRequest) (*returns.ApproveReturnResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	ret, err := h.svc.ApproveReturn(ctx, req.GetId(), req.GetReviewerId())
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.ApproveReturnResponse{
		ReturnOrder: mapModelToProtoReturn(ret),
	}, nil
}

func (h *ReturnGRPCHandler) RejectReturn(ctx context.Context, req *returns.RejectReturnRequest) (*returns.RejectReturnResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	ret, err := h.svc.RejectReturn(ctx, req.GetId(), req.GetRejectionReason(), req.GetReviewerId())
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.RejectReturnResponse{
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

func (h *ReturnGRPCHandler) CreateExchange(ctx context.Context, req *returns.CreateExchangeRequest) (*returns.CreateExchangeResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	answersDTO := make([]*dto.ExchangeAnswerDTO, len(req.GetAnswers()))
	for i, a := range req.GetAnswers() {
		answersDTO[i] = &dto.ExchangeAnswerDTO{
			QuestionKey:     a.GetQuestionKey(),
			AnswerValue:     a.GetAnswerValue(),
			DeductionAmount: a.GetDeductionAmount(),
		}
	}

	exc, err := h.svc.CreateExchange(ctx, &dto.CreateExchangeDTO{
		OrderID:           req.GetOrderId(),
		OrderItemID:       req.GetOrderItemId(),
		UserID:            req.GetUserId(),
		NewProductID:      req.GetNewProductId(),
		OldBrand:          req.GetOldBrand(),
		OldModel:          req.GetOldModel(),
		OldVariant:        req.GetOldVariant(),
		SerialOrIMEI:      req.GetSerialOrImei(),
		BaseExchangeValue: req.GetBaseExchangeValue(),
		Answers:           answersDTO,
	})
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.CreateExchangeResponse{
		Exchange: mapExchangeToProto(exc),
	}, nil
}

func (h *ReturnGRPCHandler) GetExchange(ctx context.Context, req *returns.GetExchangeRequest) (*returns.GetExchangeResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	var exc *model.Exchange
	var err error
	if req.GetId() != "" {
		exc, err = h.svc.GetExchange(ctx, req.GetId())
	} else {
		return nil, status.Error(codes.InvalidArgument, "exchange id is required")
	}

	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.GetExchangeResponse{
		Exchange: mapExchangeToProto(exc),
	}, nil
}

func (h *ReturnGRPCHandler) VerifyExchangeProduct(ctx context.Context, req *returns.VerifyExchangeProductRequest) (*returns.VerifyExchangeProductResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	exc, v, err := h.svc.VerifyExchangeProduct(ctx, &dto.VerifyExchangeProductDTO{
		ExchangeID:       req.GetExchangeId(),
		VerifiedBy:       req.GetVerifiedBy(),
		ConditionRating:  req.GetConditionRating(),
		IsApproved:       req.GetIsApproved(),
		ScreenIntact:     req.GetScreenIntact(),
		BodyIntact:       req.GetBodyIntact(),
		Functional:       req.GetFunctional(),
		SerialMatches:    req.GetSerialMatches(),
		AdjustedDiscount: req.GetAdjustedDiscount(),
		ProofImages:      req.GetProofImages(),
		Notes:            req.GetNotes(),
	})
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.VerifyExchangeProductResponse{
		Exchange:     mapExchangeToProto(exc),
		Verification: mapExchangeVerificationToProto(v),
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
	if m.RejectionReason != "" {
		ret.RejectionReason = m.RejectionReason
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

func mapExchangeToProto(exc *model.Exchange) *returns.Exchange {
	if exc == nil {
		return nil
	}

	protoAnswers := make([]*returns.ExchangeQuestionAnswer, len(exc.Answers))
	for i, a := range exc.Answers {
		protoAnswers[i] = &returns.ExchangeQuestionAnswer{
			Id:              a.ID,
			ExchangeId:      a.ExchangeID,
			QuestionKey:     a.QuestionKey,
			AnswerValue:     a.AnswerValue,
			DeductionAmount: a.DeductionAmount,
			CreatedAt:       a.CreatedAt.Format(time.RFC3339),
		}
	}

	res := &returns.Exchange{
		Id:                exc.ID,
		OrderId:           exc.OrderID,
		OrderItemId:       exc.OrderItemID,
		UserId:            exc.UserID,
		NewProductId:      exc.NewProductID,
		OldBrand:          exc.OldBrand,
		OldModel:          exc.OldModel,
		OldVariant:        exc.OldVariant,
		SerialOrImei:      exc.SerialOrIMEI,
		BaseExchangeValue: exc.BaseExchangeValue,
		EstimatedDiscount: exc.EstimatedDiscount,
		FinalDiscount:     exc.FinalDiscount,
		PriceDifference:   exc.PriceDifference,
		Status:            string(exc.Status),
		Answers:           protoAnswers,
		CreatedAt:         exc.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         exc.UpdatedAt.Format(time.RFC3339),
	}

	if exc.Verification != nil {
		res.Verification = mapExchangeVerificationToProto(exc.Verification)
	}

	return res
}

func mapExchangeVerificationToProto(v *model.ExchangeVerification) *returns.ExchangeVerification {
	if v == nil {
		return nil
	}

	return &returns.ExchangeVerification{
		Id:               v.ID,
		ExchangeId:       v.ExchangeID,
		VerifiedBy:       v.VerifiedBy,
		ConditionRating:  string(v.ConditionRating),
		IsApproved:       v.IsApproved,
		ScreenIntact:     v.ScreenIntact,
		BodyIntact:       v.BodyIntact,
		Functional:       v.Functional,
		SerialMatches:    v.SerialMatches,
		AdjustedDiscount: v.AdjustedDiscount,
		ProofImages:      v.ProofImages,
		Notes:            v.Notes,
		VerifiedAt:       v.VerifiedAt.Format(time.RFC3339),
		CreatedAt:        v.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        v.UpdatedAt.Format(time.RFC3339),
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
		errors.Is(err, service.ErrRejectionReasonRequired),
		errors.Is(err, service.ErrInvalidVerificationCondition),
		errors.Is(err, service.ErrExchangeBrandModelRequired),
		errors.Is(err, service.ErrExchangeNewProductRequired):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrDuplicateReturnRequest):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, service.ErrOrderNotDelivered),
		errors.Is(err, service.ErrReturnWindowExpired),
		errors.Is(err, service.ErrProductNotReturnable),
		errors.Is(err, service.ErrReturnNotVerifiable),
		errors.Is(err, service.ErrInvalidStateTransition),
		errors.Is(err, service.ErrExchangeNotVerifiable),
		errors.Is(err, service.ErrExchangeFunctionalRequired):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, service.ErrOrderItemNotFound),
		errors.Is(err, service.ErrOrderNotFound),
		errors.Is(err, service.ErrReturnNotFound),
		errors.Is(err, repository.ErrReturnNotFound),
		errors.Is(err, service.ErrExchangeNotFound),
		errors.Is(err, repository.ErrExchangeNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, service.ErrUnauthorizedReturnRequest),
		errors.Is(err, service.ErrCustomerCannotReviewOwnReturn):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

