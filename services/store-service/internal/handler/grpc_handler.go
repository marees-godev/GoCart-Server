package handler

import (
	"context"
	"log/slog"

	storepb "github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type StoreGRPCHandler struct {
	storepb.UnimplementedStoreServiceServer
	storeService service.StoreService
	logger       *slog.Logger
}

func NewStoreGRPCHandler(storeService service.StoreService, log ...*slog.Logger) *StoreGRPCHandler {
	var l *slog.Logger
	if len(log) > 0 {
		l = log[0]
	}
	return &StoreGRPCHandler{
		storeService: storeService,
		logger:       l,
	}
}

func extractMerchantID(ctx context.Context, fallbackID string) string {
	if user, ok := auth.UserFromContext(ctx); ok && user != nil && user.UserID != "" {
		return user.UserID
	}
	if uID := grpcclient.GetUserID(ctx); uID != "" {
		return uID
	}
	return fallbackID
}

func (h *StoreGRPCHandler) CreateStore(ctx context.Context, req *storepb.CreateStoreRequest) (*storepb.CreateStoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	merchantID := extractMerchantID(ctx, req.MerchantId)

	var bankDetails *string
	if req.BankAccountDetails != "" {
		bankDetails = &req.BankAccountDetails
	}

	createReq := dto.CreateStoreRequest{
		MerchantID:         merchantID,
		Name:               req.Name,
		Slug:               req.Slug,
		BusinessEmail:      req.BusinessEmail,
		BusinessPhone:      req.BusinessPhone,
		Description:        req.Description,
		LogoURL:            req.LogoUrl,
		Address:            req.Address,
		BankAccountDetails: bankDetails,
	}

	store, err := h.storeService.CreateStore(ctx, merchantID, createReq)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.CreateStoreResponse{
		Store: dto.ToStorePB(store),
	}, nil
}

func (h *StoreGRPCHandler) GetStore(ctx context.Context, req *storepb.GetStoreRequest) (*storepb.GetStoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	merchantID := extractMerchantID(ctx, req.MerchantId)

	store, err := h.storeService.GetStore(ctx, merchantID, req.Id)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.GetStoreResponse{
		Store: dto.ToStorePB(store),
	}, nil
}

func (h *StoreGRPCHandler) ListStores(ctx context.Context, req *storepb.ListStoresRequest) (*storepb.ListStoresResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	merchantID := req.MerchantId
	limit := int(req.Limit)
	offset := int(req.Offset)

	stores, total, err := h.storeService.ListStores(ctx, merchantID, limit, offset)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	pbStores := make([]*storepb.Store, 0, len(stores))
	for _, s := range stores {
		pbStores = append(pbStores, dto.ToStorePB(s))
	}

	return &storepb.ListStoresResponse{
		Stores: pbStores,
		Total:  int32(total),
	}, nil
}

func (h *StoreGRPCHandler) UpdateStore(ctx context.Context, req *storepb.UpdateStoreRequest) (*storepb.UpdateStoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	merchantID := extractMerchantID(ctx, req.MerchantId)

	updateReq := dto.UpdateStoreRequest{
		Name:               req.Name,
		Slug:               req.Slug,
		BusinessEmail:      req.BusinessEmail,
		BusinessPhone:      req.BusinessPhone,
		Description:        req.Description,
		LogoURL:            req.LogoUrl,
		Address:            req.Address,
		IsVacationMode:     req.IsVacationMode,
		BankAccountDetails: req.BankAccountDetails,
	}

	store, err := h.storeService.UpdateStore(ctx, merchantID, req.Id, updateReq)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.UpdateStoreResponse{
		Store: dto.ToStorePB(store),
	}, nil
}

func (h *StoreGRPCHandler) GetUploadUrl(ctx context.Context, req *storepb.GetUploadUrlRequest) (*storepb.GetUploadUrlResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	merchantID := extractMerchantID(ctx, req.MerchantId)

	uploadReq := dto.GetUploadURLRequest{
		MerchantID:  merchantID,
		ImageType:   req.ImageType,
		Filename:    req.Filename,
		ContentType: req.ContentType,
	}

	res, err := h.storeService.GetUploadURL(ctx, merchantID, uploadReq)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.GetUploadUrlResponse{
		UploadUrl:        res.UploadURL,
		PublicUrl:        res.PublicURL,
		Key:              res.Key,
		ExpiresInSeconds: res.ExpiresInSeconds,
	}, nil
}

func extractAdminID(ctx context.Context, fallbackID string) string {
	if user, ok := auth.UserFromContext(ctx); ok && user != nil && user.UserID != "" {
		return user.UserID
	}
	if uID := grpcclient.GetUserID(ctx); uID != "" {
		return uID
	}
	return fallbackID
}

func (h *StoreGRPCHandler) SubmitStore(ctx context.Context, req *storepb.SubmitStoreRequest) (*storepb.SubmitStoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	merchantID := extractMerchantID(ctx, req.MerchantId)

	submitReq := dto.SubmitStoreRequest{
		StoreID:    req.StoreId,
		MerchantID: merchantID,
	}

	store, err := h.storeService.SubmitStore(ctx, merchantID, submitReq)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.SubmitStoreResponse{
		Store: dto.ToStorePB(store),
	}, nil
}

func (h *StoreGRPCHandler) ApproveStore(ctx context.Context, req *storepb.ApproveStoreRequest) (*storepb.ApproveStoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	adminID := extractAdminID(ctx, req.AdminId)

	approveReq := dto.ApproveStoreRequest{
		StoreID: req.StoreId,
		AdminID: adminID,
	}

	store, err := h.storeService.ApproveStore(ctx, adminID, approveReq)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.ApproveStoreResponse{
		Store: dto.ToStorePB(store),
	}, nil
}

func (h *StoreGRPCHandler) RejectStore(ctx context.Context, req *storepb.RejectStoreRequest) (*storepb.RejectStoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	adminID := extractAdminID(ctx, req.AdminId)

	rejectReq := dto.RejectStoreRequest{
		StoreID: req.StoreId,
		AdminID: adminID,
		Reason:  req.RejectionReason,
	}

	store, err := h.storeService.RejectStore(ctx, adminID, rejectReq)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.RejectStoreResponse{
		Store: dto.ToStorePB(store),
	}, nil
}

func (h *StoreGRPCHandler) SubmitKYC(ctx context.Context, req *storepb.SubmitKYCRequest) (*storepb.SubmitKYCResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	merchantID := extractMerchantID(ctx, req.MerchantId)

	var routing *string
	if req.RoutingNumber != nil {
		routing = req.RoutingNumber
	}

	var gstin *string
	if req.Gstin != nil {
		gstin = req.Gstin
	}

	kycReq := dto.SubmitKYCRequest{
		StoreID:              req.StoreId,
		MerchantID:           merchantID,
		BusinessRegistration: req.BusinessRegistration,
		TaxID:                req.TaxId,
		BankName:             req.BankName,
		AccountNumber:        req.AccountNumber,
		AccountHolderName:    req.AccountHolderName,
		RoutingNumber:        routing,
		GSTIN:                gstin,
	}

	store, err := h.storeService.SubmitKYC(ctx, merchantID, kycReq)
	if err != nil {
		logger.FromContext(ctx).Error("SubmitKYC failed in store service", "error", err)
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.SubmitKYCResponse{
		Store: dto.ToStorePB(store),
	}, nil
}

func (h *StoreGRPCHandler) PublishStore(ctx context.Context, req *storepb.PublishStoreRequest) (*storepb.PublishStoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	merchantID := extractMerchantID(ctx, req.MerchantId)

	pubReq := dto.PublishStoreRequest{
		StoreID:    req.StoreId,
		MerchantID: merchantID,
	}

	store, err := h.storeService.PublishStore(ctx, merchantID, pubReq)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.PublishStoreResponse{
		Store: dto.ToStorePB(store),
	}, nil
}

func (h *StoreGRPCHandler) UnpublishStore(ctx context.Context, req *storepb.UnpublishStoreRequest) (*storepb.UnpublishStoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	merchantID := extractMerchantID(ctx, req.MerchantId)

	unpubReq := dto.UnpublishStoreRequest{
		StoreID:    req.StoreId,
		MerchantID: merchantID,
	}

	store, err := h.storeService.UnpublishStore(ctx, merchantID, unpubReq)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.UnpublishStoreResponse{
		Store: dto.ToStorePB(store),
	}, nil
}

func (h *StoreGRPCHandler) SuspendStore(ctx context.Context, req *storepb.SuspendStoreRequest) (*storepb.SuspendStoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	adminID := extractAdminID(ctx, req.AdminId)

	suspendReq := dto.SuspendStoreRequest{
		StoreID: req.StoreId,
		AdminID: adminID,
		Reason:  req.Reason,
	}

	store, err := h.storeService.SuspendStore(ctx, adminID, suspendReq)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.SuspendStoreResponse{
		Store: dto.ToStorePB(store),
	}, nil
}

func (h *StoreGRPCHandler) UnsuspendStore(ctx context.Context, req *storepb.UnsuspendStoreRequest) (*storepb.UnsuspendStoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	adminID := extractAdminID(ctx, req.AdminId)

	var reason string
	if req.Reason != nil {
		reason = *req.Reason
	}

	unsuspendReq := dto.UnsuspendStoreRequest{
		StoreID: req.StoreId,
		AdminID: adminID,
		Reason:  reason,
	}

	store, err := h.storeService.UnsuspendStore(ctx, adminID, unsuspendReq)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.UnsuspendStoreResponse{
		Store: dto.ToStorePB(store),
	}, nil
}

func (h *StoreGRPCHandler) AppealStore(ctx context.Context, req *storepb.AppealStoreRequest) (*storepb.AppealStoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	merchantID := extractMerchantID(ctx, req.MerchantId)

	appealReq := dto.AppealStoreRequest{
		StoreID:    req.StoreId,
		MerchantID: merchantID,
		Reason:     req.Reason,
	}

	store, appeal, err := h.storeService.AppealStore(ctx, merchantID, appealReq)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.AppealStoreResponse{
		Store:  dto.ToStorePB(store),
		Appeal: dto.ToStoreAppealPB(appeal),
	}, nil
}

func (h *StoreGRPCHandler) GetStoreAppeals(ctx context.Context, req *storepb.GetStoreAppealsRequest) (*storepb.GetStoreAppealsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	userID := extractMerchantID(ctx, "")

	appeals, err := h.storeService.GetStoreAppeals(ctx, userID, req.StoreId)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	pbAppeals := make([]*storepb.StoreAppeal, 0, len(appeals))
	for _, a := range appeals {
		pbAppeals = append(pbAppeals, dto.ToStoreAppealPB(a))
	}

	return &storepb.GetStoreAppealsResponse{
		Appeals: pbAppeals,
	}, nil
}

func (h *StoreGRPCHandler) CloseStore(ctx context.Context, req *storepb.CloseStoreRequest) (*storepb.CloseStoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	userID := extractMerchantID(ctx, req.MerchantId)

	closeReq := dto.CloseStoreRequest{
		StoreID:    req.StoreId,
		MerchantID: userID,
		Reason:     req.Reason,
	}

	store, err := h.storeService.CloseStore(ctx, userID, closeReq)
	if err != nil {
		return nil, grpcclient.ToGRPCError(err)
	}

	return &storepb.CloseStoreResponse{
		Store: dto.ToStorePB(store),
	}, nil
}
