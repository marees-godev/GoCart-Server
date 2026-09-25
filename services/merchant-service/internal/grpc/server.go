package grpc

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
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
		TaxId:           m.TaxID,
		Status:          m.Status,
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
		BusinessPhone: req.BusinessPhone,
		TaxID:         req.TaxId,
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
		reqStatus = req.Status
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

	s.logger.Info("gRPC UpdateMerchantStatus: updating status",
		slog.String("merchant_id", id.String()),
		slog.String("status", req.Status),
	)

	dtoReq := dto.UpdateMerchantStatusRequest{
		Status:          req.Status,
		RejectionReason: req.RejectionReason,
	}

	merchant, err := s.merchantService.UpdateMerchantStatus(ctx, id, dtoReq)
	if err != nil {
		s.logger.Warn("gRPC UpdateMerchantStatus: update failed", slog.String("merchant_id", id.String()), slog.Any("error", err))
		return nil, grpcclient.ToGRPCError(err)
	}

	return &merchantpb.UpdateMerchantStatusResponse{
		Merchant: toProtoMerchant(merchant),
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
