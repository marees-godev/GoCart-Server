package grpc

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	internalAuth "github.com/marees-godev/GoCart-Server/services/merchant-service/internal/auth"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func extractRole(ctx context.Context) string {
	if u, ok := auth.FromContext(ctx); ok && u != nil && u.Role != "" {
		return strings.ToUpper(strings.TrimSpace(u.Role))
	}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for _, key := range []string{"x-user-role", "role"} {
			if vals := md.Get(key); len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
				return strings.ToUpper(strings.TrimSpace(vals[0]))
			}
		}
	}
	return ""
}

type MerchantGRPCServer struct {
	merchantpb.UnimplementedMerchantServiceServer
	merchantService service.MerchantService
	logger          *slog.Logger
}

func NewMerchantGRPCServer(svc service.MerchantService, log ...*slog.Logger) *MerchantGRPCServer {
	var l *slog.Logger
	if len(log) > 0 && log[0] != nil {
		l = log[0]
	} else {
		l = slog.Default()
	}
	return &MerchantGRPCServer{
		merchantService: svc,
		logger:          l,
	}
}

func toProtoStatus(s string) merchantpb.MerchantStatus {
	if val, ok := merchantpb.MerchantStatus_value[strings.ToUpper(strings.TrimSpace(s))]; ok {
		return merchantpb.MerchantStatus(val)
	}
	return merchantpb.MerchantStatus_PENDING
}

func toProtoLifecycleStatus(s string) merchantpb.MerchantLifecycleStatus {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case string(model.MerchantLifecycleStatusActive):
		return merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_ACTIVE
	case string(model.MerchantLifecycleStatusInactive):
		return merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_INACTIVE
	case string(model.MerchantLifecycleStatusPendingReview):
		return merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_PENDING_REVIEW
	case string(model.MerchantLifecycleStatusSuspended):
		return merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_SUSPENDED
	case string(model.MerchantLifecycleStatusTerminated):
		return merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_TERMINATED
	default:
		return merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_UNSPECIFIED
	}
}

func toProtoMerchant(m *model.Merchant) *merchantpb.MerchantResponseData {
	if m == nil {
		return nil
	}
	res := &merchantpb.MerchantResponseData{
		Id:              m.ID.String(),
		BusinessName:    m.BusinessName,
		FirstName:       m.FirstName,
		LastName:        m.LastName,
		BusinessEmail:   m.BusinessEmail,
		BusinessPhone:   m.BusinessPhone,
		PanCardNumber:   m.PanCardNumber,
		Status:          toProtoStatus(m.Status),
		RejectionReason: m.RejectionReason,
		CreatedAt:       timestamppb.New(m.CreatedAt),
		UpdatedAt:       timestamppb.New(m.UpdatedAt),
	}
	if m.DeletedAt != nil {
		res.DeletedAt = timestamppb.New(*m.DeletedAt)
	}
	return res
}

func (s *MerchantGRPCServer) GetMerchant(ctx context.Context, req *merchantpb.GetMerchantRequest) (*merchantpb.GetMerchantResponse, error) {
	if req == nil || strings.TrimSpace(req.Id) == "" {
		s.logger.Warn("gRPC GetMerchant: missing merchant ID")
		return nil, status.Error(codes.InvalidArgument, "merchant id is required")
	}

	id, err := uuid.Parse(strings.TrimSpace(req.Id))
	if err != nil {
		s.logger.Warn("gRPC GetMerchant: invalid merchant ID format", slog.String("id", req.Id), slog.Any("error", err))
		return nil, status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	s.logger.Info("gRPC GetMerchant: fetching merchant", slog.String("merchant_id", id.String()))
	merchant, err := s.merchantService.GetMerchantByID(ctx, id)
	if err != nil {
		s.logger.Warn("gRPC GetMerchant: failed to get merchant", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, grpcclient.ToGRPCError(err)
	}

	return &merchantpb.GetMerchantResponse{
		Merchant: toProtoMerchant(merchant),
	}, nil
}

func (s *MerchantGRPCServer) UpdateMerchant(ctx context.Context, req *merchantpb.UpdateMerchantRequest) (*merchantpb.UpdateMerchantResponse, error) {
	if req == nil || strings.TrimSpace(req.Id) == "" {
		s.logger.Warn("gRPC UpdateMerchant: missing merchant ID")
		return nil, status.Error(codes.InvalidArgument, "merchant id is required")
	}

	id, err := uuid.Parse(strings.TrimSpace(req.Id))
	if err != nil {
		s.logger.Warn("gRPC UpdateMerchant: invalid merchant ID format", slog.String("id", req.Id), slog.Any("error", err))
		return nil, status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	s.logger.Info("gRPC UpdateMerchant: updating merchant details",
		slog.String("merchant_id", id.String()),
		slog.String("business_name", req.BusinessName),
	)

	dtoReq := dto.UpdateMerchantRequest{
		BusinessName:  req.BusinessName,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		BusinessPhone: req.BusinessPhone,
		PanCardNumber: req.PanCardNumber,
	}

	merchant, err := s.merchantService.UpdateMerchant(ctx, id, dtoReq)
	if err != nil {
		s.logger.Warn("gRPC UpdateMerchant: update failed", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, grpcclient.ToGRPCError(err)
	}

	return &merchantpb.UpdateMerchantResponse{
		Merchant: toProtoMerchant(merchant),
	}, nil
}

func (s *MerchantGRPCServer) CreateMerchant(ctx context.Context, req *merchantpb.CreateMerchantRequest) (*merchantpb.CreateMerchantResponse, error) {
	if req == nil {
		s.logger.Warn("gRPC CreateMerchant: nil request received")
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	role := extractRole(ctx)
	if role != "" && role != "MERCHANT" && role != "ADMIN" {
		s.logger.Warn("gRPC CreateMerchant: forbidden caller role", slog.String("role", role))
		return nil, status.Error(codes.PermissionDenied, "only users with MERCHANT role can create a merchant account")
	}

	s.logger.Info("gRPC CreateMerchant: creating merchant account",
		slog.String("id", req.Id),
		slog.String("business_email", req.BusinessEmail),
		slog.String("role", role),
	)

	dtoReq := dto.CreateMerchantRequest{
		ID:            req.Id,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		BusinessEmail: req.BusinessEmail,
	}

	merchant, err := s.merchantService.CreateMerchant(ctx, dtoReq)
	if err != nil {
		s.logger.Warn("gRPC CreateMerchant: creation failed", slog.String("id", req.Id), slog.Any("error", err))
		return nil, grpcclient.ToGRPCError(err)
	}

	return &merchantpb.CreateMerchantResponse{
		Merchant: toProtoMerchant(merchant),
	}, nil
}

func (s *MerchantGRPCServer) ListMerchants(ctx context.Context, req *merchantpb.ListMerchantsRequest) (*merchantpb.ListMerchantsResponse, error) {
	limit := 20
	offset := 0
	reqStatus := ""
	if req != nil {
		if req.Limit > 0 {
			limit = int(req.Limit)
		}
		if req.Offset >= 0 {
			offset = int(req.Offset)
		}
		if req.Status != nil {
			reqStatus = req.Status.String()
		}
		if req.LifecycleStatus != nil && *req.LifecycleStatus != merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_UNSPECIFIED {
			switch *req.LifecycleStatus {
			case merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_ACTIVE:
				reqStatus = "ACTIVE"
			case merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_INACTIVE:
				reqStatus = "INACTIVE"
			case merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_PENDING_REVIEW:
				reqStatus = "PENDING_REVIEW"
			case merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_SUSPENDED:
				reqStatus = "SUSPENDED"
			case merchantpb.MerchantLifecycleStatus_MERCHANT_LIFECYCLE_TERMINATED:
				reqStatus = "TERMINATED"
			}
		}
	}

	if req != nil && req.ReactivatedOnly != nil && *req.ReactivatedOnly {
		s.logger.Info("gRPC ListMerchants: listing reactivated merchants",
			slog.Int("limit", limit),
			slog.Int("offset", offset),
		)
		merchants, total, err := s.merchantService.ListReactivatedMerchants(ctx, limit, offset)
		if err != nil {
			s.logger.Warn("gRPC ListMerchants: query reactivated failed", slog.Any("error", err))
			return nil, grpcclient.ToGRPCError(err)
		}
		pbList := make([]*merchantpb.MerchantResponseData, len(merchants))
		for i, m := range merchants {
			pbList[i] = toProtoMerchant(m)
		}
		return &merchantpb.ListMerchantsResponse{
			Merchants: pbList,
			Total:     int32(total),
		}, nil
	}

	s.logger.Info("gRPC ListMerchants: listing merchants",
		slog.Int("limit", limit),
		slog.Int("offset", offset),
		slog.String("status", reqStatus),
	)

	merchants, total, err := s.merchantService.ListMerchants(ctx, limit, offset, reqStatus)
	if err != nil {
		s.logger.Warn("gRPC ListMerchants: query failed", slog.Any("error", err))
		return nil, grpcclient.ToGRPCError(err)
	}

	pbList := make([]*merchantpb.MerchantResponseData, len(merchants))
	for i, m := range merchants {
		pbList[i] = toProtoMerchant(m)
	}

	return &merchantpb.ListMerchantsResponse{
		Merchants: pbList,
		Total:     int32(total),
	}, nil
}

func (s *MerchantGRPCServer) UpdateMerchantStatus(ctx context.Context, req *merchantpb.UpdateMerchantStatusRequest) (*merchantpb.UpdateMerchantStatusResponse, error) {
	if req == nil || strings.TrimSpace(req.Id) == "" {
		s.logger.Warn("gRPC UpdateMerchantStatus: missing merchant ID")
		return nil, status.Error(codes.InvalidArgument, "merchant id is required")
	}

	id, err := uuid.Parse(strings.TrimSpace(req.Id))
	if err != nil {
		s.logger.Warn("gRPC UpdateMerchantStatus: invalid merchant ID format", slog.String("id", req.Id), slog.Any("error", err))
		return nil, status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	if req.Status == merchantpb.MerchantStatus_REJECTED || req.Status == merchantpb.MerchantStatus_SUSPENDED {
		if strings.TrimSpace(req.RejectionReason) == "" {
			s.logger.Warn("gRPC UpdateMerchantStatus: missing rejection reason for REJECTED or SUSPENDED status", slog.String("status", req.Status.String()))
			return nil, status.Error(codes.InvalidArgument, "rejection reason is required when rejecting or suspending a merchant")
		}
	}

	s.logger.Info("gRPC UpdateMerchantStatus: updating status",
		slog.String("merchant_id", id.String()),
		slog.String("status", req.Status.String()),
	)

	dtoReq := dto.UpdateMerchantStatusRequest{
		Status:          req.Status.String(),
		RejectionReason: req.RejectionReason,
	}

	merchant, prevStatus, err := s.merchantService.UpdateMerchantStatus(ctx, id, dtoReq)
	if err != nil {
		s.logger.Warn("gRPC UpdateMerchantStatus: update failed", slog.String("merchant_id", id.String()), slog.Any("error", err))
		var transErr *model.InvalidStateTransitionError
		if errors.As(err, &transErr) {
			return nil, status.Error(codes.FailedPrecondition, transErr.Error())
		}
		return nil, grpcclient.ToGRPCError(err)
	}

	return &merchantpb.UpdateMerchantStatusResponse{
		Merchant:       toProtoMerchant(merchant),
		PreviousStatus: toProtoStatus(prevStatus),
	}, nil
}

func (s *MerchantGRPCServer) DeleteMerchant(ctx context.Context, req *merchantpb.DeleteMerchantRequest) (*merchantpb.DeleteMerchantResponse, error) {
	if req == nil || strings.TrimSpace(req.Id) == "" {
		s.logger.Warn("gRPC DeleteMerchant: missing merchant ID")
		return nil, status.Error(codes.InvalidArgument, "merchant id is required")
	}

	id, err := uuid.Parse(strings.TrimSpace(req.Id))
	if err != nil {
		s.logger.Warn("gRPC DeleteMerchant: invalid merchant ID format", slog.String("id", req.Id), slog.Any("error", err))
		return nil, status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	s.logger.Info("gRPC DeleteMerchant: deleting merchant", slog.String("merchant_id", id.String()))
	if err := s.merchantService.DeleteMerchant(ctx, id); err != nil {
		s.logger.Warn("gRPC DeleteMerchant: deletion failed", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, grpcclient.ToGRPCError(err)
	}

	return &merchantpb.DeleteMerchantResponse{
		Success: true,
	}, nil
}

func (s *MerchantGRPCServer) ActivateMerchant(ctx context.Context, req *merchantpb.LifecycleMerchantRequest) (*merchantpb.LifecycleMerchantResponse, error) {
	if req == nil || strings.TrimSpace(req.Id) == "" {
		s.logger.Warn("gRPC ActivateMerchant: missing merchant ID")
		return nil, status.Error(codes.InvalidArgument, "merchant id is required")
	}

	id, err := uuid.Parse(strings.TrimSpace(req.Id))
	if err != nil {
		s.logger.Warn("gRPC ActivateMerchant: invalid merchant ID format", slog.String("id", req.Id), slog.Any("error", err))
		return nil, status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	caller, err := internalAuth.ExtractIdentityFromGRPC(ctx)
	if err != nil {
		s.logger.Warn("gRPC ActivateMerchant: unauthorized caller", slog.Any("error", err))
		return nil, status.Error(codes.Unauthenticated, "unauthorized: missing or invalid gateway authentication headers")
	}
	if !caller.IsAdmin() {
		s.logger.Warn("gRPC ActivateMerchant: forbidden caller role", slog.String("user_id", caller.UserID), slog.String("role", caller.Role))
		return nil, status.Error(codes.PermissionDenied, "forbidden: administrator privileges required")
	}

	s.logger.Info("gRPC ActivateMerchant: activating merchant", slog.String("merchant_id", id.String()), slog.String("admin_id", caller.UserID))
	merchant, prevStatus, err := s.merchantService.ActivateMerchant(ctx, id, req.Reason, caller.UserID, caller.RequestID)
	if err != nil {
		s.logger.Warn("gRPC ActivateMerchant: activation failed", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, grpcclient.ToGRPCError(err)
	}

	return &merchantpb.LifecycleMerchantResponse{
		Merchant:       toProtoMerchant(merchant),
		PreviousStatus: toProtoLifecycleStatus(string(prevStatus)),
		Message:        "merchant activated successfully",
	}, nil
}

func (s *MerchantGRPCServer) SuspendMerchant(ctx context.Context, req *merchantpb.LifecycleMerchantRequest) (*merchantpb.LifecycleMerchantResponse, error) {
	if req == nil || strings.TrimSpace(req.Id) == "" {
		s.logger.Warn("gRPC SuspendMerchant: missing merchant ID")
		return nil, status.Error(codes.InvalidArgument, "merchant id is required")
	}

	id, err := uuid.Parse(strings.TrimSpace(req.Id))
	if err != nil {
		s.logger.Warn("gRPC SuspendMerchant: invalid merchant ID format", slog.String("id", req.Id), slog.Any("error", err))
		return nil, status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	caller, err := internalAuth.ExtractIdentityFromGRPC(ctx)
	if err != nil {
		s.logger.Warn("gRPC SuspendMerchant: unauthorized caller", slog.Any("error", err))
		return nil, status.Error(codes.Unauthenticated, "unauthorized: missing or invalid gateway authentication headers")
	}
	if !caller.IsAdmin() {
		s.logger.Warn("gRPC SuspendMerchant: forbidden caller role", slog.String("user_id", caller.UserID), slog.String("role", caller.Role))
		return nil, status.Error(codes.PermissionDenied, "forbidden: administrator privileges required")
	}

	s.logger.Info("gRPC SuspendMerchant: suspending merchant", slog.String("merchant_id", id.String()), slog.String("admin_id", caller.UserID))
	merchant, prevStatus, err := s.merchantService.SuspendMerchant(ctx, id, req.Reason, caller.UserID, caller.RequestID)
	if err != nil {
		s.logger.Warn("gRPC SuspendMerchant: suspension failed", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, grpcclient.ToGRPCError(err)
	}

	return &merchantpb.LifecycleMerchantResponse{
		Merchant:       toProtoMerchant(merchant),
		PreviousStatus: toProtoLifecycleStatus(string(prevStatus)),
		Message:        "merchant suspended successfully",
	}, nil
}

func (s *MerchantGRPCServer) ReactivateMerchant(ctx context.Context, req *merchantpb.LifecycleMerchantRequest) (*merchantpb.LifecycleMerchantResponse, error) {
	if req == nil || strings.TrimSpace(req.Id) == "" {
		s.logger.Warn("gRPC ReactivateMerchant: missing merchant ID")
		return nil, status.Error(codes.InvalidArgument, "merchant id is required")
	}

	id, err := uuid.Parse(strings.TrimSpace(req.Id))
	if err != nil {
		s.logger.Warn("gRPC ReactivateMerchant: invalid merchant ID format", slog.String("id", req.Id), slog.Any("error", err))
		return nil, status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	caller, err := internalAuth.ExtractIdentityFromGRPC(ctx)
	if err != nil {
		s.logger.Warn("gRPC ReactivateMerchant: unauthorized caller", slog.Any("error", err))
		return nil, status.Error(codes.Unauthenticated, "unauthorized: missing or invalid gateway authentication headers")
	}
	if !caller.IsAdmin() {
		s.logger.Warn("gRPC ReactivateMerchant: forbidden caller role", slog.String("user_id", caller.UserID), slog.String("role", caller.Role))
		return nil, status.Error(codes.PermissionDenied, "forbidden: administrator privileges required")
	}

	s.logger.Info("gRPC ReactivateMerchant: reactivating merchant", slog.String("merchant_id", id.String()), slog.String("admin_id", caller.UserID))
	merchant, prevStatus, err := s.merchantService.ReactivateMerchant(ctx, id, req.Reason, caller.UserID, caller.RequestID)
	if err != nil {
		s.logger.Warn("gRPC ReactivateMerchant: reactivation failed", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, grpcclient.ToGRPCError(err)
	}

	return &merchantpb.LifecycleMerchantResponse{
		Merchant:       toProtoMerchant(merchant),
		PreviousStatus: toProtoLifecycleStatus(string(prevStatus)),
		Message:        "merchant reactivated successfully",
	}, nil
}

func (s *MerchantGRPCServer) CreateMerchantAppeal(ctx context.Context, req *merchantpb.CreateMerchantAppealRequest) (*merchantpb.CreateMerchantAppealResponse, error) {
	if req == nil || strings.TrimSpace(req.MerchantId) == "" {
		s.logger.Warn("gRPC CreateMerchantAppeal: missing merchant ID")
		return nil, status.Error(codes.InvalidArgument, "merchant id is required")
	}
	if strings.TrimSpace(req.Reason) == "" {
		s.logger.Warn("gRPC CreateMerchantAppeal: missing reason")
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}

	id, err := uuid.Parse(strings.TrimSpace(req.MerchantId))
	if err != nil {
		s.logger.Warn("gRPC CreateMerchantAppeal: invalid merchant ID format", slog.String("id", req.MerchantId), slog.Any("error", err))
		return nil, status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	appeal, err := s.merchantService.CreateAppeal(ctx, id, req.Reason)
	if err != nil {
		s.logger.Warn("gRPC CreateMerchantAppeal: failed", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, grpcclient.ToGRPCError(err)
	}

	return &merchantpb.CreateMerchantAppealResponse{
		Appeal: toProtoAppeal(appeal),
	}, nil
}

func (s *MerchantGRPCServer) GetMerchantAppeals(ctx context.Context, req *merchantpb.GetMerchantAppealsRequest) (*merchantpb.GetMerchantAppealsResponse, error) {
	if req == nil || strings.TrimSpace(req.MerchantId) == "" {
		s.logger.Warn("gRPC GetMerchantAppeals: missing merchant ID")
		return nil, status.Error(codes.InvalidArgument, "merchant id is required")
	}

	id, err := uuid.Parse(strings.TrimSpace(req.MerchantId))
	if err != nil {
		s.logger.Warn("gRPC GetMerchantAppeals: invalid merchant ID format", slog.String("id", req.MerchantId), slog.Any("error", err))
		return nil, status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	appeals, err := s.merchantService.GetAppeals(ctx, id)
	if err != nil {
		s.logger.Warn("gRPC GetMerchantAppeals: failed", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, grpcclient.ToGRPCError(err)
	}

	pbAppeals := make([]*merchantpb.MerchantAppealData, len(appeals))
	for i, a := range appeals {
		pbAppeals[i] = toProtoAppeal(a)
	}

	return &merchantpb.GetMerchantAppealsResponse{
		Appeals: pbAppeals,
	}, nil
}

func toProtoAppeal(a *model.MerchantAppeal) *merchantpb.MerchantAppealData {
	if a == nil {
		return nil
	}
	res := &merchantpb.MerchantAppealData{
		Id:         a.ID.String(),
		MerchantId: a.MerchantID.String(),
		Reason:     a.Reason,
		Status:     a.Status,
		CreatedAt:  timestamppb.New(a.CreatedAt),
		UpdatedAt:  timestamppb.New(a.UpdatedAt),
	}
	if a.AdminComment != nil {
		res.AdminComment = *a.AdminComment
	}
	if a.ReviewedAt != nil {
		res.ReviewedAt = timestamppb.New(*a.ReviewedAt)
	}
	return res
}
