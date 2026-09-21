package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/storage"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/store-service/internal/repository"
)

type StoreService interface {
	CreateStore(ctx context.Context, authMerchantID string, req dto.CreateStoreRequest) (*model.Store, error)
	GetStore(ctx context.Context, authMerchantID, storeID string) (*model.Store, error)
	GetStoreByID(ctx context.Context, storeID string) (*model.Store, error)
	GetStoreByMerchantID(ctx context.Context, merchantID string) (*model.Store, error)
	ListStores(ctx context.Context, merchantID string, limit, offset int) ([]*model.Store, int, error)
	UpdateStore(ctx context.Context, authMerchantID, storeID string, req dto.UpdateStoreRequest) (*model.Store, error)
	UploadImage(ctx context.Context, authMerchantID, imageType, filename string, data []byte, contentType string) (string, error)
	GetUploadURL(ctx context.Context, authMerchantID string, req dto.GetUploadURLRequest) (*dto.GetUploadURLResponse, error)
}

type storeService struct {
	repo     repository.StoreRepository
	uploader storage.Uploader
}

func NewStoreService(repo repository.StoreRepository, uploader ...storage.Uploader) StoreService {
	var up storage.Uploader
	if len(uploader) > 0 {
		up = uploader[0]
	}
	return &storeService{
		repo:     repo,
		uploader: up,
	}
}

func resolveMerchantID(ctx context.Context, fallbackID string) string {
	if user, ok := auth.UserFromContext(ctx); ok && user != nil && user.UserID != "" {
		return user.UserID
	}
	return strings.TrimSpace(fallbackID)
}

func (s *storeService) processImage(ctx context.Context, folder, input string) string {
	if s.uploader == nil || !strings.HasPrefix(input, "data:image/") {
		return input
	}

	parts := strings.SplitN(input, ",", 2)
	if len(parts) != 2 {
		return input
	}

	header := parts[0] // e.g. "data:image/png;base64"
	b64Data := parts[1]

	contentType := "image/png"
	if strings.Contains(header, "image/jpeg") || strings.Contains(header, "image/jpg") {
		contentType = "image/jpeg"
	} else if strings.Contains(header, "image/webp") {
		contentType = "image/webp"
	} else if strings.Contains(header, "image/svg+xml") {
		contentType = "image/svg+xml"
	}

	ext := ".png"
	if contentType == "image/jpeg" {
		ext = ".jpg"
	} else if contentType == "image/webp" {
		ext = ".webp"
	} else if contentType == "image/svg+xml" {
		ext = ".svg"
	}

	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return input
	}

	key := s.uploader.GenerateKey(folder, "image"+ext)
	uploadedURL, err := s.uploader.UploadBytes(ctx, key, decoded, contentType)
	if err != nil {
		return input
	}

	return uploadedURL
}

func (s *storeService) CreateStore(ctx context.Context, authMerchantID string, req dto.CreateStoreRequest) (*model.Store, error) {
	merchantID := resolveMerchantID(ctx, authMerchantID)
	if merchantID == "" {
		merchantID = strings.TrimSpace(req.MerchantID)
	}
	if merchantID == "" {
		return nil, errors.Unauthorized("missing authenticated merchant context")
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	baseSlug := dto.GenerateSlug(req.GetName())
	slug := baseSlug
	suffix := 1

	for {
		available, err := s.repo.IsSlugAvailable(ctx, slug, "")
		if err != nil {
			return nil, err
		}
		if available {
			break
		}
		slug = fmt.Sprintf("%s-%d", baseSlug, suffix)
		suffix++
	}

	logoURL := s.processImage(ctx, "logos", req.GetLogo())
	bannerURL := s.processImage(ctx, "banners", req.GetBanner())

	store := &model.Store{
		MerchantID:         merchantID,
		Name:               req.GetName(),
		Slug:               slug,
		Description:        req.GetDescription(),
		LogoURL:            logoURL,
		BannerURL:          bannerURL,
		Address:            strings.TrimSpace(req.Address),
		ApprovalStatus:     "PENDING",
		PublishStatus:      false,
		KYCStatus:          "PENDING",
		BankAccountDetails: req.BankAccountDetails,
		AvgStoreRating:     0.0,
	}

	if err := s.repo.Create(ctx, store); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *storeService) GetStore(ctx context.Context, authMerchantID, storeID string) (*model.Store, error) {
	resolvedMerchantID := resolveMerchantID(ctx, authMerchantID)

	if strings.TrimSpace(storeID) != "" {
		return s.repo.GetByID(ctx, strings.TrimSpace(storeID))
	}

	if resolvedMerchantID != "" {
		return s.repo.GetByMerchantID(ctx, resolvedMerchantID)
	}

	return nil, errors.BadRequest("store ID or merchant context is required")
}

func (s *storeService) GetStoreByID(ctx context.Context, storeID string) (*model.Store, error) {
	trimmedID := strings.TrimSpace(storeID)
	if trimmedID == "" {
		return nil, errors.BadRequest("store ID is required")
	}
	return s.repo.GetByID(ctx, trimmedID)
}

func (s *storeService) GetStoreByMerchantID(ctx context.Context, merchantID string) (*model.Store, error) {
	trimmedID := strings.TrimSpace(merchantID)
	if trimmedID == "" {
		return nil, errors.BadRequest("merchant ID is required")
	}
	return s.repo.GetByMerchantID(ctx, trimmedID)
}

func (s *storeService) ListStores(ctx context.Context, merchantID string, limit, offset int) ([]*model.Store, int, error) {
	return s.repo.List(ctx, strings.TrimSpace(merchantID), limit, offset)
}

func (s *storeService) UpdateStore(ctx context.Context, authMerchantID, storeID string, req dto.UpdateStoreRequest) (*model.Store, error) {
	merchantID := resolveMerchantID(ctx, authMerchantID)
	if merchantID == "" {
		return nil, errors.Unauthorized("missing authenticated merchant context")
	}

	targetStoreID := strings.TrimSpace(storeID)
	if targetStoreID == "" && req.StoreID != nil {
		targetStoreID = strings.TrimSpace(*req.StoreID)
	}
	if targetStoreID == "" && req.ID != nil {
		targetStoreID = strings.TrimSpace(*req.ID)
	}

	var existingStore *model.Store
	var err error

	if targetStoreID != "" {
		existingStore, err = s.repo.GetByID(ctx, targetStoreID)
	} else {
		existingStore, err = s.repo.GetByMerchantID(ctx, merchantID)
	}

	if err != nil {
		return nil, err
	}
	if existingStore == nil {
		return nil, errors.NotFound("store not found")
	}

	// Enforce merchant ownership
	if existingStore.MerchantID != merchantID {
		return nil, errors.Forbidden("merchant cannot modify another merchant's store")
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	if name := req.GetName(); name != nil && *name != "" && *name != existingStore.Name {
		existingStore.Name = *name
		baseSlug := dto.GenerateSlug(*name)
		slug := baseSlug
		suffix := 1
		for {
			available, err := s.repo.IsSlugAvailable(ctx, slug, existingStore.ID)
			if err != nil {
				return nil, err
			}
			if available {
				break
			}
			slug = fmt.Sprintf("%s-%d", baseSlug, suffix)
			suffix++
		}
		existingStore.Slug = slug
	}

	if desc := req.GetDescription(); desc != nil {
		existingStore.Description = strings.TrimSpace(*desc)
	}

	if logo := req.GetLogo(); logo != nil {
		existingStore.LogoURL = s.processImage(ctx, "logos", strings.TrimSpace(*logo))
	}

	if banner := req.GetBanner(); banner != nil {
		existingStore.BannerURL = s.processImage(ctx, "banners", strings.TrimSpace(*banner))
	}

	if req.Address != nil {
		existingStore.Address = strings.TrimSpace(*req.Address)
	}

	if req.PublishStatus != nil {
		existingStore.PublishStatus = *req.PublishStatus
	}

	if req.BankAccountDetails != nil {
		existingStore.BankAccountDetails = req.BankAccountDetails
	}

	if err := s.repo.Update(ctx, existingStore); err != nil {
		return nil, err
	}

	return existingStore, nil
}

func (s *storeService) UploadImage(ctx context.Context, authMerchantID, imageType, filename string, data []byte, contentType string) (string, error) {
	merchantID := resolveMerchantID(ctx, authMerchantID)
	if merchantID == "" {
		return "", errors.Unauthorized("missing authenticated merchant context")
	}

	if s.uploader == nil {
		return "", errors.Internal(nil, "storage uploader is not configured")
	}

	folder := "stores"
	if imageType == "logo" || imageType == "logos" {
		folder = "logos"
	} else if imageType == "banner" || imageType == "banners" {
		folder = "banners"
	}

	key := s.uploader.GenerateKey(folder, filename)
	return s.uploader.UploadBytes(ctx, key, data, contentType)
}

func (s *storeService) GetUploadURL(ctx context.Context, authMerchantID string, req dto.GetUploadURLRequest) (*dto.GetUploadURLResponse, error) {
	merchantID := resolveMerchantID(ctx, authMerchantID)
	if merchantID == "" {
		merchantID = strings.TrimSpace(req.MerchantID)
	}
	if merchantID == "" {
		return nil, errors.Unauthorized("missing authenticated merchant context")
	}

	if s.uploader == nil {
		return nil, errors.Internal(nil, "storage uploader is not configured")
	}

	folder := "stores"
	imageType := strings.ToLower(strings.TrimSpace(req.ImageType))
	if imageType == "logo" || imageType == "logos" {
		folder = "logos"
	} else if imageType == "banner" || imageType == "banners" {
		folder = "banners"
	}

	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		filename = "image.png"
	}

	contentType := strings.TrimSpace(req.ContentType)
	if contentType == "" {
		contentType = "image/png"
	}

	key := s.uploader.GenerateKey(folder, filename)
	uploadURL, err := s.uploader.GetPresignedPutURL(ctx, key, contentType, 15*time.Minute)
	if err != nil {
		return nil, err
	}

	publicURL := s.uploader.GetPublicURL(key)

	return &dto.GetUploadURLResponse{
		UploadURL:        uploadURL,
		PublicURL:        publicURL,
		Key:              key,
		ExpiresInSeconds: 900,
	}, nil
}
