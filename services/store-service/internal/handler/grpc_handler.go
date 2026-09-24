package handler

import (
	"context"

	storepb "github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type StoreGRPCHandler struct {
	storepb.UnimplementedStoreServiceServer
	storeService service.StoreService
}

func NewStoreGRPCHandler(storeService service.StoreService) *StoreGRPCHandler {
	return &StoreGRPCHandler{
		storeService: storeService,
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
