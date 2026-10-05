package service

import (
	"context"
	"encoding/base64"
	"log/slog"
	"strings"

	categorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/category"
	storepb "github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/storage"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/repository"
	"google.golang.org/grpc"
)

type StoreClient interface {
	GetStore(ctx context.Context, in *storepb.GetStoreRequest, opts ...grpc.CallOption) (*storepb.GetStoreResponse, error)
}

type CategoryClient interface {
	GetCategory(ctx context.Context, in *categorypb.GetCategoryRequest, opts ...grpc.CallOption) (*categorypb.GetCategoryResponse, error)
}

type ProductService interface {
	CreateProduct(ctx context.Context, req dto.CreateProductRequest) (*model.Product, error)
	GetProduct(ctx context.Context, id string) (*model.Product, error)
	ListProducts(ctx context.Context, req dto.ListProductsRequest) ([]*model.Product, int32, error)
	UpdateProduct(ctx context.Context, req dto.UpdateProductRequest) (*model.Product, error)
	DeleteProduct(ctx context.Context, id string) error
}

type productService struct {
	repo           repository.ProductRepository
	storeClient    StoreClient
	categoryClient CategoryClient
	uploader       storage.Uploader
	logger         *slog.Logger
}

func NewProductService(
	repo repository.ProductRepository,
	storeClient StoreClient,
	categoryClient CategoryClient,
	uploader storage.Uploader,
	log ...*slog.Logger,
) ProductService {
	var l *slog.Logger
	if len(log) > 0 {
		l = log[0]
	}
	return &productService{
		repo:           repo,
		storeClient:    storeClient,
		categoryClient: categoryClient,
		uploader:       uploader,
		logger:         l,
	}
}

func (s *productService) processImageDataURL(ctx context.Context, input string, folder string) string {
	if s.uploader == nil || !strings.HasPrefix(input, "data:") {
		return input
	}
	parts := strings.SplitN(input, ";base64,", 2)
	if len(parts) != 2 {
		return input
	}
	header := parts[0]
	b64Data := parts[1]

	contentType := "image/png"
	if strings.Contains(header, "image/jpeg") || strings.Contains(header, "image/jpg") {
		contentType = "image/jpeg"
	} else if strings.Contains(header, "image/webp") {
		contentType = "image/webp"
	} else if strings.Contains(header, "image/svg+xml") {
		contentType = "image/svg+xml"
	}

	var ext string
	switch contentType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	case "image/svg+xml":
		ext = ".svg"
	default:
		ext = ".png"
	}

	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return input
	}

	key := s.uploader.GenerateKey(folder, "product"+ext)
	uploadedURL, err := s.uploader.UploadBytes(ctx, key, decoded, contentType)
	if err != nil {
		return input
	}

	return uploadedURL
}

func (s *productService) CreateProduct(ctx context.Context, req dto.CreateProductRequest) (*model.Product, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	user, hasUser := auth.UserFromContext(ctx)
	if hasUser && user.Role == auth.RoleMerchant {
		if s.storeClient != nil {
			stResp, err := s.storeClient.GetStore(ctx, &storepb.GetStoreRequest{Id: req.StoreID})
			if err != nil || stResp == nil || stResp.Store == nil {
				return nil, appErrors.InvalidArgument("invalid or non-existent store_id")
			}
			if stResp.Store.MerchantId != user.UserID {
				return nil, appErrors.Forbidden("merchant does not own this store")
			}
		}
	}

	if s.categoryClient != nil {
		catResp, err := s.categoryClient.GetCategory(ctx, &categorypb.GetCategoryRequest{Id: req.CategoryID})
		if err != nil || catResp == nil || catResp.Category == nil || catResp.Category.Id == "" {
			return nil, appErrors.InvalidArgument("invalid or non-existent category_id")
		}
		if !catResp.Category.IsActive {
			return nil, appErrors.InvalidArgument("referenced category is inactive")
		}
	}

	existing, err := s.repo.GetProductBySKU(ctx, req.StoreID, req.SKU)
	if err == nil && existing != nil {
		return nil, appErrors.AlreadyExists("SKU already exists in this store")
	}

	name := req.Name

	imgURL := s.processImageDataURL(ctx, req.ImageURL, "products")

	images := make([]model.ProductImage, 0, len(req.Images))
	for i, raw := range req.Images {
		u := s.processImageDataURL(ctx, raw, "products")
		images = append(images, model.ProductImage{
			URL:          u,
			IsPrimary:    i == 0,
			DisplayOrder: i,
		})
	}

	variants := make([]model.ProductVariant, 0, len(req.Variants))
	for _, v := range req.Variants {
		variants = append(variants, model.ProductVariant{
			SKU:            v.SKU,
			Name:           v.Name,
			Price:          v.Price,
			MRP:            v.MRP,
			Stock:          v.Stock,
			AttributesJSON: v.AttributesJSON,
			Status:         model.StatusInStock,
		})
	}

	if imgURL == "" && len(images) > 0 {
		imgURL = images[0].URL
	}

	product := &model.Product{
		StoreID:     req.StoreID,
		CategoryID:  req.CategoryID,
		SKU:         req.SKU,
		Name:        name,
		Description: req.Description,
		Price:       req.Price,
		MRP:         req.MRP,
		Tax:         req.Tax,
		Currency:    "USD",
		Status:      model.ProductStatus(req.Status),
		ImageURL:    imgURL,
		AvgRating:   0.00,
		Images:      images,
		Variants:    variants,
	}

	if err := s.repo.CreateProduct(ctx, product); err != nil {
		return nil, err
	}

	return product, nil
}

func (s *productService) GetProduct(ctx context.Context, id string) (*model.Product, error) {
	if id == "" {
		return nil, appErrors.InvalidArgument("product id is required")
	}
	return s.repo.GetProductByID(ctx, id)
}

func (s *productService) ListProducts(ctx context.Context, req dto.ListProductsRequest) ([]*model.Product, int32, error) {
	return s.repo.ListProducts(ctx, req)
}

func (s *productService) UpdateProduct(ctx context.Context, req dto.UpdateProductRequest) (*model.Product, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetProductByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	user, hasUser := auth.UserFromContext(ctx)
	if hasUser && user.Role == auth.RoleMerchant {
		if s.storeClient != nil {
			stResp, err := s.storeClient.GetStore(ctx, &storepb.GetStoreRequest{Id: existing.StoreID})
			if err != nil || stResp == nil || stResp.Store == nil {
				return nil, appErrors.Forbidden("merchant does not own this store")
			}
			if stResp.Store.MerchantId != user.UserID {
				return nil, appErrors.Forbidden("merchant does not own this product's store")
			}
		}
	}

	if req.CategoryID != nil && *req.CategoryID != existing.CategoryID {
		if s.categoryClient != nil {
			catResp, err := s.categoryClient.GetCategory(ctx, &categorypb.GetCategoryRequest{Id: *req.CategoryID})
			if err != nil || catResp == nil || catResp.Category == nil || catResp.Category.Id == "" {
				return nil, appErrors.InvalidArgument("invalid or non-existent category_id")
			}
			if !catResp.Category.IsActive {
				return nil, appErrors.InvalidArgument("referenced category is inactive")
			}
		}
		existing.CategoryID = *req.CategoryID
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Price != nil {
		existing.Price = *req.Price
	}
	if req.MRP != nil {
		existing.MRP = *req.MRP
	}
	if req.Tax != nil {
		existing.Tax = *req.Tax
	}
	if req.Status != nil {
		existing.Status = model.ProductStatus(*req.Status)
	}
	if req.ImageURL != nil {
		existing.ImageURL = s.processImageDataURL(ctx, *req.ImageURL, "products")
	}

	if len(req.Images) > 0 {
		newImages := make([]model.ProductImage, 0, len(req.Images))
		for i, raw := range req.Images {
			u := s.processImageDataURL(ctx, raw, "products")
			newImages = append(newImages, model.ProductImage{
				ProductID:    existing.ID,
				URL:          u,
				IsPrimary:    i == 0,
				DisplayOrder: i,
			})
		}
		existing.Images = newImages
		if req.ImageURL == nil && len(newImages) > 0 {
			existing.ImageURL = newImages[0].URL
		}
	}

	if len(req.Variants) > 0 {
		newVariants := make([]model.ProductVariant, 0, len(req.Variants))
		for _, v := range req.Variants {
			vStatus := model.ProductStatus(v.Status)
			if vStatus == "" {
				vStatus = model.StatusInStock
			}
			newVariants = append(newVariants, model.ProductVariant{
				ID:             v.ID,
				ProductID:      existing.ID,
				SKU:            v.SKU,
				Name:           v.Name,
				Price:          v.Price,
				MRP:            v.MRP,
				Stock:          v.Stock,
				AttributesJSON: v.AttributesJSON,
				Status:         vStatus,
			})
		}
		existing.Variants = newVariants
	}

	if err := s.repo.UpdateProduct(ctx, existing); err != nil {
		return nil, err
	}

	return existing, nil
}

func (s *productService) DeleteProduct(ctx context.Context, id string) error {
	if id == "" {
		return appErrors.InvalidArgument("product id is required")
	}

	existing, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		return err
	}

	user, hasUser := auth.UserFromContext(ctx)
	if hasUser && user.Role == auth.RoleMerchant {
		if s.storeClient != nil {
			stResp, err := s.storeClient.GetStore(ctx, &storepb.GetStoreRequest{Id: existing.StoreID})
			if err != nil || stResp == nil || stResp.Store == nil {
				return appErrors.Forbidden("merchant does not own this store")
			}
			if stResp.Store.MerchantId != user.UserID {
				return appErrors.Forbidden("merchant does not own this product's store")
			} 
			
		}
	}

	return s.repo.DeleteProduct(ctx, id)
}
