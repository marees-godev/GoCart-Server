package handler

import (
	"context"
	"log/slog"
	"strings"

	categorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/category"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/category-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CategoryGRPCHandler struct {
	categorypb.UnimplementedCategoryServiceServer
	categoryService service.CategoryService
	logger          *slog.Logger
}

func NewCategoryGRPCHandler(categoryService service.CategoryService, log ...*slog.Logger) *CategoryGRPCHandler {
	var l *slog.Logger
	if len(log) > 0 {
		l = log[0]
	}
	return &CategoryGRPCHandler{
		categoryService: categoryService,
		logger:          l,
	}
}

func (h *CategoryGRPCHandler) CreateCategory(ctx context.Context, req *categorypb.CreateCategoryRequest) (*categorypb.CreateCategoryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	parentID := strings.TrimSpace(req.ParentCategoryId)

	var parentIDPtr *string
	if parentID != "" {
		parentIDPtr = &parentID
	}

	createReq := dto.CreateCategoryRequest{
		Name:             req.Name,
		ParentCategoryID: parentIDPtr,
		Description:      req.Description,
	}

	cat, err := h.categoryService.CreateCategory(ctx, createReq)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("CreateCategory failed", "error", err)
		}
		return nil, grpcclient.ToGRPCError(err)
	}

	if h.logger != nil {
		h.logger.Info("CreateCategory RPC succeeded", "id", cat.ID)
	}

	return &categorypb.CreateCategoryResponse{
		Category: dto.ToCategoryPB(cat),
	}, nil
}

func (h *CategoryGRPCHandler) GetCategory(ctx context.Context, req *categorypb.GetCategoryRequest) (*categorypb.GetCategoryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	cat, err := h.categoryService.GetCategory(ctx, req.Id)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("GetCategory failed", "error", err)
		}
		return nil, grpcclient.ToGRPCError(err)
	}

	if h.logger != nil {
		h.logger.Debug("GetCategory RPC succeeded", "id", cat.ID)
	}

	return &categorypb.GetCategoryResponse{
		Category: dto.ToCategoryPB(cat),
	}, nil
}

func (h *CategoryGRPCHandler) ListCategories(ctx context.Context, req *categorypb.ListCategoriesRequest) (*categorypb.ListCategoriesResponse, error) {
	if req == nil {
		req = &categorypb.ListCategoriesRequest{}
	}

	var parentIDPtr *string
	if p := strings.TrimSpace(req.ParentCategoryId); p != "" {
		parentIDPtr = &p
	}

	listReq := dto.ListCategoriesRequest{
		Limit:            req.Limit,
		Offset:           req.Offset,
		ParentCategoryID: parentIDPtr,
		RootOnly:         req.RootOnly,
	}

	categories, total, err := h.categoryService.ListCategories(ctx, listReq)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("ListCategories failed", "error", err)
		}
		return nil, grpcclient.ToGRPCError(err)
	}

	if h.logger != nil {
		h.logger.Debug("ListCategories RPC succeeded", "total", total)
	}

	return &categorypb.ListCategoriesResponse{
		Categories: dto.ToCategoryListPB(categories),
		Total:      total,
	}, nil
}

func (h *CategoryGRPCHandler) GetChildCategories(ctx context.Context, req *categorypb.GetChildCategoriesRequest) (*categorypb.GetChildCategoriesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	childReq := dto.GetChildCategoriesRequest{
		ParentCategoryID: req.ParentCategoryId,
		Limit:            req.Limit,
		Offset:           req.Offset,
	}

	categories, total, err := h.categoryService.GetChildCategories(ctx, childReq)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("GetChildCategories failed", "error", err)
		}
		return nil, grpcclient.ToGRPCError(err)
	}

	if h.logger != nil {
		h.logger.Debug("GetChildCategories RPC succeeded", "parent_id", req.ParentCategoryId, "total", total)
	}

	return &categorypb.GetChildCategoriesResponse{
		Categories: dto.ToCategoryListPB(categories),
		Total:      total,
	}, nil
}

func (h *CategoryGRPCHandler) UpdateCategory(ctx context.Context, req *categorypb.UpdateCategoryRequest) (*categorypb.UpdateCategoryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	updateReq := dto.UpdateCategoryRequest{
		ID: req.Id,
	}

	if req.Name != "" {
		updateReq.Name = &req.Name
	}
	if req.Description != "" {
		updateReq.Description = &req.Description
	}
	if req.ParentCategoryId != "" {
		updateReq.ParentCategoryID = &req.ParentCategoryId
	}
	if req.IsActive != nil {
		updateReq.IsActive = req.IsActive
	}

	cat, err := h.categoryService.UpdateCategory(ctx, updateReq)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("UpdateCategory failed", "error", err)
		}
		return nil, grpcclient.ToGRPCError(err)
	}

	if h.logger != nil {
		h.logger.Info("UpdateCategory RPC succeeded", "id", cat.ID)
	}

	return &categorypb.UpdateCategoryResponse{
		Category: dto.ToCategoryPB(cat),
	}, nil
}

func (h *CategoryGRPCHandler) DeleteCategory(ctx context.Context, req *categorypb.DeleteCategoryRequest) (*categorypb.DeleteCategoryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	if err := h.categoryService.DeleteCategory(ctx, req.Id); err != nil {
		if h.logger != nil {
			h.logger.Error("DeleteCategory failed", "error", err)
		}
		return nil, grpcclient.ToGRPCError(err)
	}

	if h.logger != nil {
		h.logger.Info("DeleteCategory RPC succeeded", "id", req.Id)
	}

	return &categorypb.DeleteCategoryResponse{
		Success: true,
		Message: "Category deleted successfully",
	}, nil
}
