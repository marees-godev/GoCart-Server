package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"

	categorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/category"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
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

type MerchantClient interface {
	GetMerchant(ctx context.Context, in *merchantpb.GetMerchantRequest, opts ...grpc.CallOption) (*merchantpb.GetMerchantResponse, error)
}

type ProductService interface {
	CreateProduct(ctx context.Context, req dto.CreateProductRequest) (*model.Product, error)
	GetProduct(ctx context.Context, id string) (*model.Product, error)
	ListProducts(ctx context.Context, req dto.ListProductsRequest) ([]*model.Product, int32, error)
	UpdateProduct(ctx context.Context, req dto.UpdateProductRequest) (*model.Product, error)
	DeleteProduct(ctx context.Context, id string) error
	DeleteProductVariant(ctx context.Context, productID, variantID string) error
	ReportProduct(ctx context.Context, req dto.ReportProductRequest) (*model.ProductReport, error)
	ListProductReports(ctx context.Context, req dto.ListProductReportsRequest) ([]*model.ProductReport, int32, error)
}

type productService struct {
	repo           repository.ProductRepository
	storeClient    StoreClient
	categoryClient CategoryClient
	merchantClient MerchantClient
	uploader       storage.Uploader
	stateValidator *model.StateTransitionValidator
	logger         *slog.Logger
}

func NewProductService(
	repo repository.ProductRepository,
	storeClient StoreClient,
	categoryClient CategoryClient,
	uploader storage.Uploader,
	log ...*slog.Logger,
) ProductService {
	return NewProductServiceWithClients(repo, storeClient, categoryClient, nil, uploader, log...)
}

func NewProductServiceWithClients(
	repo repository.ProductRepository,
	storeClient StoreClient,
	categoryClient CategoryClient,
	merchantClient MerchantClient,
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
		merchantClient: merchantClient,
		uploader:       uploader,
		stateValidator: model.NewStateTransitionValidator(),
		logger:         l,
	}
}

func (s *productService) checkPublishPrerequisites(ctx context.Context, storeID string, p *model.Product) error {
	if err := p.ValidateForPublishing(); err != nil {
		return err
	}

	var merchantID string
	if s.storeClient != nil {
		stResp, err := s.storeClient.GetStore(ctx, &storepb.GetStoreRequest{Id: storeID})
		if err != nil || stResp == nil || stResp.Store == nil {
			return appErrors.InvalidArgument("invalid or non-existent store_id")
		}
		appStatus := strings.ToUpper(strings.TrimSpace(stResp.Store.ApprovalStatus))
		if appStatus != "APPROVED" {
			return appErrors.Conflict(fmt.Sprintf("cannot publish product: store approval status must be APPROVED (current: '%s')", stResp.Store.ApprovalStatus))
		}
		merchantID = stResp.Store.MerchantId
	}

	if s.merchantClient != nil && merchantID != "" {
		mResp, err := s.merchantClient.GetMerchant(ctx, &merchantpb.GetMerchantRequest{Id: merchantID})
		if err != nil || mResp == nil || mResp.Merchant == nil {
			return appErrors.InvalidArgument("invalid or non-existent merchant for store")
		}
		mStatus := strings.ToUpper(strings.TrimSpace(mResp.Merchant.Status.String()))
		if mStatus != "ACTIVE" && mStatus != "APPROVED" {
			return appErrors.Conflict(fmt.Sprintf("cannot publish product: merchant status must be ACTIVE (current: '%s')", mResp.Merchant.Status.String()))
		}
	}

	return nil
}

func (s *productService) processImageDataURL(ctx context.Context, input string, folder string) (string, error) {
	if !strings.HasPrefix(input, "data:") {
		return input, nil
	}

	if s.uploader == nil {
		return "", appErrors.Internal(nil, "storage service is not configured or unavailable")
	}

	parts := strings.SplitN(input, ";base64,", 2)
	if len(parts) != 2 {
		return "", appErrors.BadRequest("invalid image data URL format")
	}
	header := parts[0]
	b64Data := parts[1]

	var contentType, ext string
	switch {
	case strings.Contains(header, "image/jpeg") || strings.Contains(header, "image/jpg"):
		contentType, ext = "image/jpeg", ".jpg"
	case strings.Contains(header, "image/png"):
		contentType, ext = "image/png", ".png"
	case strings.Contains(header, "image/webp"):
		contentType, ext = "image/webp", ".webp"
	case strings.Contains(header, "image/svg+xml"):
		contentType, ext = "image/svg+xml", ".svg"
	default:
		return "", appErrors.BadRequest("unsupported image type: only JPEG, PNG, WebP, and SVG are allowed")
	}

	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return "", appErrors.BadRequest("failed to decode base64 image data")
	}

	key := s.uploader.GenerateKey(folder, "product"+ext)
	uploadedURL, err := s.uploader.UploadBytes(ctx, key, decoded, contentType)
	if err != nil {
		return "", appErrors.Internal(err, "failed to upload image to storage")
	}

	return uploadedURL, nil
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

	imgURL, err := s.processImageDataURL(ctx, req.ImageURL, "products")
	if err != nil {
		return nil, err
	}

	images := make([]model.ProductImage, 0, len(req.Images))
	for i, raw := range req.Images {
		u, err := s.processImageDataURL(ctx, raw, "products")
		if err != nil {
			return nil, err
		}
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

	prodStatus := model.ProductStatus(req.Status)
	if prodStatus == "" {
		prodStatus = model.StatusDraft
	}
	if !prodStatus.IsValid() {
		return nil, appErrors.BadRequest("invalid product status")
	}

	product := &model.Product{
		StoreID:             req.StoreID,
		CategoryID:          req.CategoryID,
		SKU:                 req.SKU,
		Name:                name,
		Description:         req.Description,
		Price:               req.Price,
		MRP:                 req.MRP,
		Tax:                 req.Tax,
		Currency:            "USD",
		Status:              prodStatus,
		ImageURL:            imgURL,
		AvgRating:           0.00,
		Weight:      req.Weight,
		IsExchangeEligible:  req.IsExchangeEligible,
		MaxExchangeDiscount: req.MaxExchangeDiscount,
		Images:              images,
		Variants:            variants,
	}

	if prodStatus == model.StatusPublished {
		if err := s.checkPublishPrerequisites(ctx, req.StoreID, product); err != nil {
			return nil, err
		}
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
	if req.Weight != nil {
		existing.Weight = *req.Weight
	}

	if req.Status != nil {
		targetStatus := model.ProductStatus(*req.Status)
		if !targetStatus.IsValid() {
			return nil, appErrors.BadRequest("invalid product status")
		}
		if err := s.stateValidator.Validate(existing.Status, targetStatus); err != nil {
			return nil, err
		}
		existing.Status = targetStatus
	}

	if req.ImageURL != nil {
		u, err := s.processImageDataURL(ctx, *req.ImageURL, "products")
		if err != nil {
			return nil, err
		}
		existing.ImageURL = u
	}

	if req.IsExchangeEligible != nil {
		existing.IsExchangeEligible = *req.IsExchangeEligible
	}
	if req.MaxExchangeDiscount != nil {
		existing.MaxExchangeDiscount = *req.MaxExchangeDiscount
	}

	if len(req.Images) > 0 {
		newImages := make([]model.ProductImage, 0, len(req.Images))
		for i, raw := range req.Images {
			u, err := s.processImageDataURL(ctx, raw, "products")
			if err != nil {
				return nil, err
			}
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

	if existing.Status == model.StatusPublished {
		if err := s.checkPublishPrerequisites(ctx, existing.StoreID, existing); err != nil {
			return nil, err
		}
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

func (s *productService) DeleteProductVariant(ctx context.Context, productID, variantID string) error {
	if productID == "" {
		return appErrors.InvalidArgument("product id is required")
	}
	if variantID == "" {
		return appErrors.InvalidArgument("variant id is required")
	}

	existing, err := s.repo.GetProductByID(ctx, productID)
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

	return s.repo.DeleteProductVariant(ctx, productID, variantID)
}

func (s *productService) ReportProduct(ctx context.Context, req dto.ReportProductRequest) (*model.ProductReport, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	normalizedReason, err := model.NormalizeReportReason(req.Reason)
	if err != nil {
		return nil, err
	}

	prod, err := s.repo.GetProductByID(ctx, req.ProductID)
	if err != nil {
		return nil, err
	}
	if prod == nil {
		return nil, appErrors.NotFound("product not found")
	}

	if prod.DeletedAt != nil || prod.Status == model.StatusDeleted {
		return nil, appErrors.BadRequest("cannot report a deleted product")
	}
	if prod.Status == model.StatusDraft {
		return nil, appErrors.BadRequest("cannot report an unpublished draft product")
	}

	existing, err := s.repo.GetProductReportByUserAndProduct(ctx, req.ProductID, req.ReporterID)
	if err == nil && existing != nil {
		return nil, appErrors.Conflict("you have already reported this product")
	}

	report := &model.ProductReport{
		ProductID:   req.ProductID,
		ReporterID:  req.ReporterID,
		Reason:      normalizedReason,
		Description: req.Description,
		Status:      model.ReportStatusPending,
	}

	if err := s.repo.CreateProductReport(ctx, report); err != nil {
		return nil, err
	}

	return report, nil
}

func (s *productService) ListProductReports(ctx context.Context, req dto.ListProductReportsRequest) ([]*model.ProductReport, int32, error) {
	if req.Status != "" {
		st := model.ReportStatus(strings.ToUpper(strings.TrimSpace(req.Status)))
		if !st.IsValid() {
			return nil, 0, appErrors.BadRequest("invalid report status filter")
		}
		req.Status = string(st)
	}
	return s.repo.ListProductReports(ctx, req)
}
