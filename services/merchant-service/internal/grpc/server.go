package grpc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
}

func NewMerchantGRPCServer(svc service.MerchantService) *MerchantGRPCServer {
	return &MerchantGRPCServer{merchantService: svc}
}

func toProtoMerchant(m *model.Merchant) *merchantpb.Merchant {
	if m == nil {
		return nil
	}
	return &merchantpb.Merchant{
		Id:              m.ID.String(),
		UserId:          m.UserID.String(),
		BusinessName:    m.BusinessName,
		Status:          m.Status,
		TaxId:           m.TaxID,
		BusinessEmail:   m.BusinessEmail,
		BusinessPhone:   m.BusinessPhone,
		RejectionReason: m.RejectionReason,
		CreatedAt:       m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       m.UpdatedAt.Format(time.RFC3339),
		FirstName:       m.FirstName,
		LastName:        m.LastName,
	}
}

func toGRPCError(err error) error {
	if err == nil {
		return nil
	}
	var appErr *appErrors.AppError
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case appErrors.CodeNotFound:
			return status.Error(codes.NotFound, appErr.Message)
		case appErrors.CodeBadRequest:
			return status.Error(codes.InvalidArgument, appErr.Message)
		case appErrors.CodeConflict:
			return status.Error(codes.AlreadyExists, appErr.Message)
		case appErrors.CodeUnauthorized:
			return status.Error(codes.Unauthenticated, appErr.Message)
		case appErrors.CodeForbidden:
			return status.Error(codes.PermissionDenied, appErr.Message)
		default:
			return status.Error(codes.Internal, appErr.Message)
		}
	}
	return status.Error(codes.Internal, err.Error())
}

func (s *MerchantGRPCServer) GetMerchant(ctx context.Context, req *merchantpb.GetMerchantRequest) (*merchantpb.GetMerchantResponse, error) {
	if req == nil || req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "merchant id is required")
	}

	id, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	merchant, err := s.merchantService.GetMerchantByID(ctx, id)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &merchantpb.GetMerchantResponse{
		Merchant: toProtoMerchant(merchant),
	}, nil
}

func (s *MerchantGRPCServer) GetMerchantByUserID(ctx context.Context, req *merchantpb.GetMerchantByUserIDRequest) (*merchantpb.GetMerchantResponse, error) {
	if req == nil || req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user id is required")
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user id format")
	}

	merchant, err := s.merchantService.GetMerchantByUserID(ctx, userID)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &merchantpb.GetMerchantResponse{
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

	merchants, total, err := s.merchantService.ListMerchants(ctx, limit, offset, reqStatus)
	if err != nil {
		return nil, toGRPCError(err)
	}

	pbList := make([]*merchantpb.Merchant, len(merchants))
	for i, m := range merchants {
		pbList[i] = toProtoMerchant(m)
	}

	return &merchantpb.ListMerchantsResponse{
		Merchants: pbList,
		Total:     int32(total),
	}, nil
}

func (s *MerchantGRPCServer) CreateMerchant(ctx context.Context, req *merchantpb.CreateMerchantRequest) (*merchantpb.CreateMerchantResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	role := extractRole(ctx)
	if role != "" && role != "MERCHANT" && role != "ADMIN" {
		return nil, status.Error(codes.PermissionDenied, "only users with MERCHANT role can create a merchant account")
	}

	dtoReq := dto.CreateMerchantRequest{
		UserID:        req.UserId,
		BusinessName:  req.BusinessName,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		TaxID:         req.TaxId,
		BusinessEmail: req.BusinessEmail,
		BusinessPhone: req.BusinessPhone,
	}

	merchant, err := s.merchantService.CreateMerchant(ctx, dtoReq)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &merchantpb.CreateMerchantResponse{
		Merchant: toProtoMerchant(merchant),
	}, nil
}

func (s *MerchantGRPCServer) UpdateMerchant(ctx context.Context, req *merchantpb.UpdateMerchantRequest) (*merchantpb.UpdateMerchantResponse, error) {
	if req == nil || req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "merchant id is required")
	}

	id, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	dtoReq := dto.UpdateMerchantRequest{
		BusinessName:  req.BusinessName,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		BusinessEmail: req.BusinessEmail,
		BusinessPhone: req.BusinessPhone,
		TaxID:         req.TaxId,
	}

	merchant, err := s.merchantService.UpdateMerchant(ctx, id, dtoReq)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &merchantpb.UpdateMerchantResponse{
		Merchant: toProtoMerchant(merchant),
	}, nil
}

func (s *MerchantGRPCServer) UpdateMerchantStatus(ctx context.Context, req *merchantpb.UpdateMerchantStatusRequest) (*merchantpb.UpdateMerchantStatusResponse, error) {
	if req == nil || req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "merchant id is required")
	}

	id, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	dtoReq := dto.UpdateMerchantStatusRequest{
		Status:          req.Status,
		RejectionReason: req.RejectionReason,
	}

	merchant, err := s.merchantService.UpdateMerchantStatus(ctx, id, dtoReq)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &merchantpb.UpdateMerchantStatusResponse{
		Merchant: toProtoMerchant(merchant),
	}, nil
}

func (s *MerchantGRPCServer) DeleteMerchant(ctx context.Context, req *merchantpb.DeleteMerchantRequest) (*merchantpb.DeleteMerchantResponse, error) {
	if req == nil || req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "merchant id is required")
	}

	id, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	if err := s.merchantService.DeleteMerchant(ctx, id); err != nil {
		return nil, toGRPCError(err)
	}

	return &merchantpb.DeleteMerchantResponse{
		Success: true,
	}, nil
}
