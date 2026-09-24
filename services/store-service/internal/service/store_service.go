package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
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
	SubmitStore(ctx context.Context, authMerchantID string, req dto.SubmitStoreRequest) (*model.Store, error)
	ApproveStore(ctx context.Context, authAdminID string, req dto.ApproveStoreRequest) (*model.Store, error)
	RejectStore(ctx context.Context, authAdminID string, req dto.RejectStoreRequest) (*model.Store, error)
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

func resolveRoleContext(ctx context.Context, fallbackID, requiredRole string) (string, error) {
	userID := ""
	role := ""
	if user, ok := auth.UserFromContext(ctx); ok && user != nil {
		userID = strings.TrimSpace(user.UserID)
		role = strings.ToUpper(strings.TrimSpace(user.Role))
	}
	if userID == "" {
		userID = grpcclient.GetUserID(ctx)
	}
	if role == "" {
		role = strings.ToUpper(strings.TrimSpace(grpcclient.GetUserRole(ctx)))
	}
	if userID == "" {
		userID = strings.TrimSpace(fallbackID)
	}

	if role == "" && userID == "" {
		return "", errors.Unauthorized("missing authenticated context")
	}
	if role != "" && role != requiredRole {
		return "", errors.Forbidden("insufficient permissions for this operation")
	}
	if userID == "" {
		return "", errors.Unauthorized("missing user identity")
	}

	return userID, nil
}

func resolveMerchantContext(ctx context.Context, fallbackID string) (string, error) {
	return resolveRoleContext(ctx, fallbackID, auth.RoleMerchant)
}

func resolveAdminContext(ctx context.Context, fallbackAdminID string) (string, error) {
	return resolveRoleContext(ctx, fallbackAdminID, auth.RoleAdmin)
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
	merchantID, err := resolveMerchantContext(ctx, authMerchantID)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	var slug string
	if req.Slug != nil && strings.TrimSpace(*req.Slug) != "" {
		customSlug := dto.GenerateSlug(*req.Slug)
		available, err := s.repo.IsSlugAvailable(ctx, customSlug, "")
		if err != nil {
			return nil, err
		}
		if !available {
			return nil, errors.Conflict("store username / slug is already taken")
		}
		slug = customSlug
	} else {
		baseSlug := dto.GenerateSlug(req.GetName())
		slug = baseSlug
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
	}

	logoURL := s.processImage(ctx, "logos", req.GetLogo())

	store := &model.Store{
		MerchantID:         merchantID,
		Name:               req.GetName(),
		Slug:               slug,
		BusinessEmail:      strings.TrimSpace(req.BusinessEmail),
		BusinessPhone:      strings.TrimSpace(req.BusinessPhone),
		Description:        req.GetDescription(),
		LogoURL:            logoURL,
		Address:            strings.TrimSpace(req.Address),
		IsVacationMode:     false,
		ApprovalStatus:     model.StoreStatusDraft,
		BankAccountDetails: req.BankAccountDetails,
		AvgStoreRating:     0.0,
	}

	if err := s.repo.Create(ctx, store); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *storeService) GetStore(ctx context.Context, authMerchantID, storeID string) (*model.Store, error) {
	resolvedMerchantID := ""
	if user, ok := auth.UserFromContext(ctx); ok && user != nil && user.UserID != "" {
		resolvedMerchantID = user.UserID
	} else if uID := grpcclient.GetUserID(ctx); uID != "" {
		resolvedMerchantID = uID
	} else {
		resolvedMerchantID = strings.TrimSpace(authMerchantID)
	}

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
	merchantID, err := resolveMerchantContext(ctx, authMerchantID)
	if err != nil {
		return nil, err
	}

	targetStoreID := strings.TrimSpace(storeID)
	if targetStoreID == "" && req.StoreID != nil {
		targetStoreID = strings.TrimSpace(*req.StoreID)
	}
	if targetStoreID == "" && req.ID != nil {
		targetStoreID = strings.TrimSpace(*req.ID)
	}

	var existingStore *model.Store

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

	if req.Slug != nil && strings.TrimSpace(*req.Slug) != "" {
		customSlug := dto.GenerateSlug(*req.Slug)
		if customSlug != existingStore.Slug {
			available, err := s.repo.IsSlugAvailable(ctx, customSlug, existingStore.ID)
			if err != nil {
				return nil, err
			}
			if !available {
				return nil, errors.Conflict("store username / slug is already taken")
			}
			existingStore.Slug = customSlug
		}
	} else if name := req.GetName(); name != nil && *name != "" && *name != existingStore.Name {
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

	if name := req.GetName(); name != nil && *name != "" {
		existingStore.Name = *name
	}

	if req.BusinessEmail != nil {
		existingStore.BusinessEmail = strings.TrimSpace(*req.BusinessEmail)
	}

	if req.BusinessPhone != nil {
		existingStore.BusinessPhone = strings.TrimSpace(*req.BusinessPhone)
	}

	if desc := req.GetDescription(); desc != nil {
		existingStore.Description = strings.TrimSpace(*desc)
	}

	if logo := req.GetLogo(); logo != nil {
		existingStore.LogoURL = s.processImage(ctx, "logos", strings.TrimSpace(*logo))
	}

	if req.Address != nil {
		existingStore.Address = strings.TrimSpace(*req.Address)
	}

	if req.IsVacationMode != nil {
		existingStore.IsVacationMode = *req.IsVacationMode
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
	if _, err := resolveMerchantContext(ctx, authMerchantID); err != nil {
		return "", err
	}

	if s.uploader == nil {
		return "", errors.Internal(nil, "storage uploader is not configured")
	}

	folder := "logos"
	key := s.uploader.GenerateKey(folder, filename)
	return s.uploader.UploadBytes(ctx, key, data, contentType)
}

func (s *storeService) GetUploadURL(ctx context.Context, authMerchantID string, req dto.GetUploadURLRequest) (*dto.GetUploadURLResponse, error) {
	if _, err := resolveMerchantContext(ctx, authMerchantID); err != nil {
		return nil, err
	}

	if s.uploader == nil {
		return nil, errors.Internal(nil, "storage uploader is not configured")
	}

	folder := "logos"
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

func (s *storeService) SubmitStore(ctx context.Context, authMerchantID string, req dto.SubmitStoreRequest) (*model.Store, error) {
	merchantID, err := resolveMerchantContext(ctx, authMerchantID)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	existingStore, err := s.repo.GetByID(ctx, strings.TrimSpace(req.StoreID))
	if err != nil {
		return nil, err
	}
	if existingStore == nil {
		return nil, errors.NotFound("store not found")
	}

	if existingStore.MerchantID != merchantID {
		return nil, errors.Forbidden("merchant cannot submit another merchant's store")
	}

	if existingStore.ApprovalStatus != model.StoreStatusDraft {
		return nil, errors.UnprocessableEntity(fmt.Sprintf("invalid state transition: store in status %s cannot be submitted for approval (must be %s)", existingStore.ApprovalStatus, model.StoreStatusDraft))
	}

	return s.repo.UpdateStatus(ctx, existingStore.ID, model.StoreStatusDraft, model.StoreStatusPendingApproval, nil)
}

func (s *storeService) ApproveStore(ctx context.Context, authAdminID string, req dto.ApproveStoreRequest) (*model.Store, error) {
	adminID, err := resolveAdminContext(ctx, authAdminID)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	existingStore, err := s.repo.GetByID(ctx, strings.TrimSpace(req.StoreID))
	if err != nil {
		return nil, err
	}
	if existingStore == nil {
		return nil, errors.NotFound("store not found")
	}

	if existingStore.MerchantID == adminID {
		return nil, errors.Forbidden("merchant cannot approve their own store")
	}

	if existingStore.ApprovalStatus != model.StoreStatusPendingApproval {
		return nil, errors.UnprocessableEntity(fmt.Sprintf("invalid state transition: store in status %s cannot be approved (must be %s)", existingStore.ApprovalStatus, model.StoreStatusPendingApproval))
	}

	return s.repo.UpdateStatus(ctx, existingStore.ID, model.StoreStatusPendingApproval, model.StoreStatusApproved, nil)
}

func (s *storeService) RejectStore(ctx context.Context, authAdminID string, req dto.RejectStoreRequest) (*model.Store, error) {
	adminID, err := resolveAdminContext(ctx, authAdminID)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return nil, errors.BadRequest("rejection reason is mandatory")
	}

	existingStore, err := s.repo.GetByID(ctx, strings.TrimSpace(req.StoreID))
	if err != nil {
		return nil, err
	}
	if existingStore == nil {
		return nil, errors.NotFound("store not found")
	}

	if existingStore.MerchantID == adminID {
		return nil, errors.Forbidden("merchant cannot reject their own store")
	}

	if existingStore.ApprovalStatus != model.StoreStatusPendingApproval {
		return nil, errors.UnprocessableEntity(fmt.Sprintf("invalid state transition: store in status %s cannot be rejected (must be %s)", existingStore.ApprovalStatus, model.StoreStatusPendingApproval))
	}

	return s.repo.UpdateStatus(ctx, existingStore.ID, model.StoreStatusPendingApproval, model.StoreStatusRejected, &reason)
}

