package handler

import (
	"context"
	"log/slog"

	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ProductGRPCHandler struct {
	productpb.UnimplementedProductServiceServer
	productService service.ProductService
	logger         *slog.Logger
}

func NewProductGRPCHandler(productService service.ProductService, log ...*slog.Logger) *ProductGRPCHandler {
	var l *slog.Logger
	if len(log) > 0 {
		l = log[0]
	}
	return &ProductGRPCHandler{
		productService: productService,
		logger:         l,
	}
}

func (h *ProductGRPCHandler) CreateProduct(ctx context.Context, req *productpb.CreateProductRequest) (*productpb.CreateProductResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	name := req.Name
	if name == "" && req.Productname != "" {
		name = req.Productname
	}

	variants := make([]dto.CreateVariantRequest, 0, len(req.Variants))
	for _, v := range req.Variants {
		variants = append(variants, dto.CreateVariantRequest{
			SKU:            v.Sku,
			Name:           v.Name,
			Price:          v.Price,
			MRP:            v.Mrp,
			Stock:          int(v.Stock),
			AttributesJSON: v.AttributesJson,
		})
	}

	createReq := dto.CreateProductRequest{
		StoreID:     req.StoreId,
		CategoryID:  req.CategoryId,
		SKU:         req.Sku,
		Name:        name,
		Description: req.Description,
		Price:       req.Price,
		MRP:         req.Mrp,
		Tax:         req.Tax,
		Status:      req.Status,
		ImageURL:    req.ImageUrl,
		Images:      req.Images,
		Variants:    variants,
	}

	prod, err := h.productService.CreateProduct(ctx, createReq)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("CreateProduct failed", "error", err)
		}
		return nil, grpcclient.ToGRPCError(err)
	}

	if h.logger != nil {
		h.logger.Info("CreateProduct RPC succeeded", "id", prod.ID)
	}

	return &productpb.CreateProductResponse{
		Product: dto.ToProductPB(prod),
	}, nil
}

func (h *ProductGRPCHandler) GetProduct(ctx context.Context, req *productpb.GetProductRequest) (*productpb.GetProductResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	prod, err := h.productService.GetProduct(ctx, req.Id)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("GetProduct failed", "error", err)
		}
		return nil, grpcclient.ToGRPCError(err)
	}

	return &productpb.GetProductResponse{
		Product: dto.ToProductPB(prod),
	}, nil
}

func (h *ProductGRPCHandler) ListProducts(ctx context.Context, req *productpb.ListProductsRequest) (*productpb.ListProductsResponse, error) {
	if req == nil {
		req = &productpb.ListProductsRequest{}
	}

	listReq := dto.ListProductsRequest{
		Limit:      req.Limit,
		Offset:     req.Offset,
		CategoryID: req.CategoryId,
		StoreID:    req.StoreId,
		Status:     req.Status,
	}

	products, total, err := h.productService.ListProducts(ctx, listReq)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("ListProducts failed", "error", err)
		}
		return nil, grpcclient.ToGRPCError(err)
	}

	return &productpb.ListProductsResponse{
		Products: dto.ToProductListPB(products),
		Total:    total,
	}, nil
}

func (h *ProductGRPCHandler) UpdateProduct(ctx context.Context, req *productpb.UpdateProductRequest) (*productpb.UpdateProductResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	updateReq := dto.UpdateProductRequest{
		ID:         req.Id,
		CategoryID: req.CategoryId,
		Images:     req.Images,
	}

	if req.Name != nil {
		updateReq.Name = req.Name
	} else if req.Productname != nil {
		updateReq.Name = req.Productname
	}

	if req.Description != nil {
		updateReq.Description = req.Description
	}
	if req.Price != nil {
		updateReq.Price = req.Price
	}
	if req.Mrp != nil {
		updateReq.MRP = req.Mrp
	}
	if req.Tax != nil {
		updateReq.Tax = req.Tax
	}
	if req.Status != nil {
		updateReq.Status = req.Status
	}
	if req.ImageUrl != nil {
		updateReq.ImageURL = req.ImageUrl
	}

	if len(req.Variants) > 0 {
		updateReq.Variants = make([]dto.UpdateVariantRequest, 0, len(req.Variants))
		for _, v := range req.Variants {
			updateReq.Variants = append(updateReq.Variants, dto.UpdateVariantRequest{
				ID:             v.Id,
				SKU:            v.Sku,
				Name:           v.Name,
				Price:          v.Price,
				MRP:            v.Mrp,
				Stock:          int(v.Stock),
				AttributesJSON: v.AttributesJson,
				Status:         v.Status,
			})
		}
	}

	prod, err := h.productService.UpdateProduct(ctx, updateReq)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("UpdateProduct failed", "error", err)
		}
		return nil, grpcclient.ToGRPCError(err)
	}

	if h.logger != nil {
		h.logger.Info("UpdateProduct RPC succeeded", "id", prod.ID)
	}

	return &productpb.UpdateProductResponse{
		Product: dto.ToProductPB(prod),
	}, nil
}

func (h *ProductGRPCHandler) DeleteProduct(ctx context.Context, req *productpb.DeleteProductRequest) (*productpb.DeleteProductResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	if err := h.productService.DeleteProduct(ctx, req.Id); err != nil {
		if h.logger != nil {
			h.logger.Error("DeleteProduct failed", "error", err)
		}
		return nil, grpcclient.ToGRPCError(err)
	}

	if h.logger != nil {
		h.logger.Info("DeleteProduct RPC succeeded", "id", req.Id)
	}

	return &productpb.DeleteProductResponse{
		Success: true,
		Message: "Product deleted successfully",
	}, nil
}
