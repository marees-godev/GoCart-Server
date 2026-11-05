package handler

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	returns "github.com/marees-godev/GoCart-Server/contracts/protobuf/return"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
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

	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "return id is required")
	}

	reviewerID := strings.TrimSpace(req.GetReviewerId())
	if reviewerID == "" {
		reviewerID = grpcclient.GetUserID(ctx)
		if reviewerID == "" {
			if u, ok := auth.UserFromContext(ctx); ok && u != nil {
				reviewerID = u.UserID
			}
		}
	}
	if reviewerID == "" {
		return nil, status.Error(codes.InvalidArgument, "reviewer id is required")
	}

	ctx = auth.WithUser(ctx, &auth.UserContext{UserID: reviewerID})

	ret, err := h.svc.ApproveReturn(ctx, id, reviewerID)
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

	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "return id is required")
	}

	reason := strings.TrimSpace(req.GetRejectionReason())
	if reason == "" {
		return nil, status.Error(codes.InvalidArgument, "rejection reason is required")
	}

	reviewerID := strings.TrimSpace(req.GetReviewerId())
	if reviewerID == "" {
		reviewerID = grpcclient.GetUserID(ctx)
		if reviewerID == "" {
			if u, ok := auth.UserFromContext(ctx); ok && u != nil {
				reviewerID = u.UserID
			}
		}
	}
	if reviewerID == "" {
		return nil, status.Error(codes.InvalidArgument, "reviewer id is required")
	}

	ctx = auth.WithUser(ctx, &auth.UserContext{UserID: reviewerID})

	ret, err := h.svc.RejectReturn(ctx, id, reason, reviewerID)
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
		ProductID:               req.GetProductId(),
		ExchangeDeviceCatalogID: req.GetExchangeDeviceCatalogId(),
		UserID:                  req.GetUserId(),
		OldBrand:                req.GetOldBrand(),
		OldModel:                req.GetOldModel(),
		OldVariant:              req.GetOldVariant(),
		SerialOrIMEI:            req.GetSerialOrImei(),
		Answers:                 answersDTO,
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
		AdjustedDiscount: req.AdjustedDiscount,
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

func (h *ReturnGRPCHandler) GetExchangeQuestions(ctx context.Context, req *returns.GetExchangeQuestionsRequest) (*returns.GetExchangeQuestionsResponse, error) {
	if req == nil {
		req = &returns.GetExchangeQuestionsRequest{}
	}

	productID := ""
	if req.ProductId != nil {
		productID = *req.ProductId
	}

	questions, devices, err := h.svc.GetExchangeQuestions(ctx, &dto.GetExchangeQuestionsDTO{
		CategoryCode: req.GetCategoryCode(),
		ProductID:    productID,
	})
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	protoQuestions := make([]*returns.ExchangeDiagnosticQuestion, 0, len(questions))
	for _, q := range questions {
		protoOpts := make([]*returns.ExchangeQuestionOption, 0, len(q.Options))
		for _, opt := range q.Options {
			protoOpts = append(protoOpts, &returns.ExchangeQuestionOption{
				Id:                  opt.ID,
				QuestionId:          opt.QuestionID,
				OptionKey:           opt.OptionKey,
				OptionLabel:         opt.OptionLabel,
				DeductionAmount:     opt.DeductionAmount,
				IsFatalDisqualifier: opt.IsFatalDisqualifier,
				RejectionReason:     opt.RejectionReason,
				DisplayOrder:        int32(opt.DisplayOrder),
			})
		}
		protoQuestions = append(protoQuestions, &returns.ExchangeDiagnosticQuestion{
			Id:           q.ID,
			CategoryCode: q.CategoryCode,
			QuestionKey:  q.QuestionKey,
			QuestionText: q.QuestionText,
			DisplayOrder: int32(q.DisplayOrder),
			Options:      protoOpts,
		})
	}

	protoDevices := make([]*returns.ExchangeDeviceOption, 0, len(devices))
	for _, d := range devices {
		protoDevices = append(protoDevices, &returns.ExchangeDeviceOption{
			Id:                d.ID,
			CategoryCode:      d.CategoryCode,
			Brand:             d.Brand,
			Model:             d.Model,
			Variant:           d.Variant,
			BaseExchangeValue: d.BaseExchangeValue,
		})
	}

	return &returns.GetExchangeQuestionsResponse{
		Questions: protoQuestions,
		Devices:   protoDevices,
	}, nil
}

func (h *ReturnGRPCHandler) CalculateExchangeValuation(ctx context.Context, req *returns.CalculateExchangeValuationRequest) (*returns.CalculateExchangeValuationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	dtoAnswers := make([]*dto.ExchangeUserAnswerDTO, 0, len(req.GetAnswers()))
	for _, a := range req.GetAnswers() {
		if a != nil {
			dtoAnswers = append(dtoAnswers, &dto.ExchangeUserAnswerDTO{
				QuestionKey: a.GetQuestionKey(),
				OptionKey:   a.GetOptionKey(),
			})
		}
	}

	valRes, err := h.svc.CalculateExchangeValuation(ctx, &dto.CalculateExchangeValuationDTO{
		ProductID:    req.GetProductId(),
		CategoryCode: req.GetCategoryCode(),
		OldBrand:     req.GetOldBrand(),
		OldModel:     req.GetOldModel(),
		OldVariant:   req.GetOldVariant(),
		Answers:      dtoAnswers,
	})
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	protoDeductions := make([]*returns.ExchangeDeductionDetail, 0, len(valRes.Deductions))
	for _, d := range valRes.Deductions {
		protoDeductions = append(protoDeductions, &returns.ExchangeDeductionDetail{
			QuestionKey:     d.QuestionKey,
			OptionKey:       d.OptionKey,
			DeductionAmount: d.DeductionAmount,
			IsFatal:         d.IsFatal,
			RejectionReason: d.RejectionReason,
		})
	}

	return &returns.CalculateExchangeValuationResponse{
		IsEligible:        valRes.IsEligible,
		RejectionReason:   valRes.RejectionReason,
		BaseExchangeValue: valRes.BaseExchangeValue,
		TotalDeductions:   valRes.TotalDeductions,
		FinalDiscount:     valRes.FinalDiscount,
		Deductions:        protoDeductions,
	}, nil
}

func (h *ReturnGRPCHandler) LockExchangeQuote(ctx context.Context, req *returns.LockExchangeQuoteRequest) (*returns.LockExchangeQuoteResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	dtoAnswers := make([]*dto.ExchangeUserAnswerDTO, 0, len(req.GetAnswers()))
	for _, a := range req.GetAnswers() {
		if a != nil {
			dtoAnswers = append(dtoAnswers, &dto.ExchangeUserAnswerDTO{
				QuestionKey: a.GetQuestionKey(),
				OptionKey:   a.GetOptionKey(),
			})
		}
	}

	quote, err := h.svc.LockExchangeQuote(ctx, &dto.LockExchangeQuoteDTO{
		UserID:       req.GetUserId(),
		ProductID:    req.GetProductId(),
		CategoryCode: req.GetCategoryCode(),
		OldBrand:     req.GetOldBrand(),
		OldModel:     req.GetOldModel(),
		OldVariant:   req.GetOldVariant(),
		SerialOrIMEI: req.GetSerialOrImei(),
		Answers:      dtoAnswers,
	})
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.LockExchangeQuoteResponse{
		Quote: mapExchangeQuoteToProto(quote),
	}, nil
}

func (h *ReturnGRPCHandler) GetExchangeQuote(ctx context.Context, req *returns.GetExchangeQuoteRequest) (*returns.GetExchangeQuoteResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	quote, err := h.svc.GetExchangeQuote(ctx, req.GetId())
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.GetExchangeQuoteResponse{
		Quote: mapExchangeQuoteToProto(quote),
	}, nil
}

func (h *ReturnGRPCHandler) ApplyExchangeToOrder(ctx context.Context, req *returns.ApplyExchangeToOrderRequest) (*returns.ApplyExchangeToOrderResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	exc, err := h.svc.ApplyExchangeToOrder(ctx, &dto.ApplyExchangeToOrderDTO{
		QuoteID:     req.GetQuoteId(),
		OrderID:     req.GetOrderId(),
		OrderItemID: req.GetOrderItemId(),
		UserID:      req.GetUserId(),
	})
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.ApplyExchangeToOrderResponse{
		Exchange: mapExchangeToProto(exc),
	}, nil
}

func (h *ReturnGRPCHandler) ResolveExchangeMismatch(ctx context.Context, req *returns.ResolveExchangeMismatchRequest) (*returns.ResolveExchangeMismatchResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	exc, err := h.svc.ResolveExchangeMismatch(ctx, &dto.ResolveExchangeMismatchDTO{
		ExchangeID:    req.GetExchangeId(),
		Action:        req.GetAction(),
		PaymentMethod: req.GetPaymentMethod(),
		Notes:         req.GetNotes(),
	})
	if err != nil {
		return nil, mapErrorToGRPC(err)
	}

	return &returns.ResolveExchangeMismatchResponse{
		Exchange: mapExchangeToProto(exc),
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
		Id:                      exc.ID,
		UserId:                  exc.UserID,
		OldBrand:          exc.OldBrand,
		OldModel:          exc.OldModel,
		OldVariant:        exc.OldVariant,
		SerialOrImei:      exc.SerialOrIMEI,
		BaseExchangeValue: exc.BaseExchangeValue,
		EstimatedDiscount: exc.EstimatedDiscount,
		FinalDiscount:     exc.FinalDiscount,
		PriceDifference:   exc.PriceDifference,
		Status:                  string(exc.Status),
		QuoteId:                 exc.QuoteID,
		ProductId:               exc.ProductID,
		ExchangeDeviceCatalogId: exc.ExchangeDeviceCatalogID,
		Answers:                 protoAnswers,
		CreatedAt:         exc.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         exc.UpdatedAt.Format(time.RFC3339),
	}

	if exc.Verification != nil {
		res.Verification = mapExchangeVerificationToProto(exc.Verification)
	}

	return res
}

func mapExchangeQuoteToProto(q *model.ExchangeQuote) *returns.ExchangeQuote {
	if q == nil {
		return nil
	}

	protoAnswers := make([]*returns.ExchangeQuoteAnswer, 0, len(q.Answers))
	for _, a := range q.Answers {
		protoAnswers = append(protoAnswers, &returns.ExchangeQuoteAnswer{
			Id:              a.ID,
			QuoteId:         a.QuoteID,
			QuestionKey:     a.QuestionKey,
			OptionKey:       a.OptionKey,
			DeductionAmount: a.DeductionAmount,
			IsFatal:         a.IsFatal,
		})
	}

	return &returns.ExchangeQuote{
		Id:                     q.ID,
		UserId:                 q.UserID,
		ProductId:              q.ProductID,
		CategoryCode:           q.CategoryCode,
		OldBrand:               q.OldBrand,
		OldModel:               q.OldModel,
		OldVariant:             q.OldVariant,
		SerialOrImei:           q.SerialOrIMEI,
		BaseExchangeValue:      q.BaseExchangeValue,
		TotalDeductions:        q.TotalDeductions,
		FinalDiscount:          q.FinalDiscount,
		IsDisqualified:         q.IsDisqualified,
		DisqualificationReason: q.DisqualificationReason,
		Status:                 string(q.Status),
		ExpiresAt:              q.ExpiresAt.Format(time.RFC3339),
		Answers:                protoAnswers,
		CreatedAt:              q.CreatedAt.Format(time.RFC3339),
		UpdatedAt:              q.UpdatedAt.Format(time.RFC3339),
	}
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
		errors.Is(err, service.ErrReturnIDRequired),
		errors.Is(err, service.ErrReviewerIDRequired),
		errors.Is(err, service.ErrOrderItemRequired),
		errors.Is(err, service.ErrOrderIDRequired),
		errors.Is(err, service.ErrUserIDRequired),
		errors.Is(err, service.ErrInvalidReturnStatus),
		errors.Is(err, service.ErrInvalidReturnType),
		errors.Is(err, service.ErrMultipleItemsNotSupported),
		errors.Is(err, service.ErrOrderItemMismatch),
		errors.Is(err, service.ErrVerifiedByRequired),
		errors.Is(err, service.ErrRejectionReasonRequired),
		errors.Is(err, service.ErrInvalidVerificationCondition),
		errors.Is(err, service.ErrInvalidExchangeConditionRating),
		errors.Is(err, service.ErrExchangeBrandModelRequired),
		errors.Is(err, service.ErrExchangeProductIDRequired),
		errors.Is(err, service.ErrExchangeDeviceCatalogIDRequired),
		errors.Is(err, service.ErrExchangeProductRequired),
		errors.Is(err, service.ErrUnknownExchangeQuestion),
		errors.Is(err, service.ErrUnknownExchangeOption),
		errors.Is(err, service.ErrDuplicateExchangeAnswer),
		errors.Is(err, service.ErrMissingRequiredQuestion):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrDuplicateReturnRequest):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, service.ErrOrderNotDelivered),
		errors.Is(err, service.ErrReturnWindowExpired),
		errors.Is(err, service.ErrProductNotReturnable),
		errors.Is(err, service.ErrProductNotExchangeEligible),
		errors.Is(err, service.ErrProductCategoryNotExchangeable),
		errors.Is(err, service.ErrSmallProductExchangeRestricted),
		errors.Is(err, service.ErrExchangeDeviceInactive),
		errors.Is(err, service.ErrIncompatibleExchangeCategory),
		errors.Is(err, service.ErrReturnNotVerifiable),
		errors.Is(err, service.ErrInvalidStateTransition),
		errors.Is(err, service.ErrExchangeNotVerifiable),
		errors.Is(err, service.ErrExchangeFunctionalRequired),
		errors.Is(err, service.ErrExchangeQuoteExpired),
		errors.Is(err, service.ErrExchangeQuoteNotApplicable),
		errors.Is(err, repository.ErrQuoteNotApplicable),
		errors.Is(err, service.ErrExchangeDisqualified):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, service.ErrOrderItemNotFound),
		errors.Is(err, service.ErrOrderNotFound),
		errors.Is(err, service.ErrReturnNotFound),
		errors.Is(err, repository.ErrReturnNotFound),
		errors.Is(err, service.ErrExchangeNotFound),
		errors.Is(err, repository.ErrExchangeNotFound),
		errors.Is(err, service.ErrExchangeQuoteNotFound),
		errors.Is(err, repository.ErrExchangeQuoteNotFound),
		errors.Is(err, service.ErrExchangeDeviceNotFound),
		errors.Is(err, service.ErrProductNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, service.ErrUnauthorizedReturnRequest),
		errors.Is(err, service.ErrCustomerCannotReviewOwnReturn):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, service.ErrProductServiceUnavailable):
		return status.Error(codes.Unavailable, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}