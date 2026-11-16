package service

import (
	"context"
	"errors"
	"testing"

	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
	"github.com/gofrs/uuid/v5"
	"google.golang.org/grpc"
)

type mockFacebookVerifier struct {
	payload     *FacebookTokenPayload
	verifyErr   error
	exchanged   string
	exchangeErr error
}

func (m *mockFacebookVerifier) Verify(ctx context.Context, token string) (*FacebookTokenPayload, error) {
	if m.verifyErr != nil {
		return nil, m.verifyErr
	}
	return m.payload, nil
}

func (m *mockFacebookVerifier) ExchangeCode(ctx context.Context, code, redirectURI string) (string, error) {
	if m.exchangeErr != nil {
		return "", m.exchangeErr
	}
	if m.exchanged != "" {
		return m.exchanged, nil
	}
	return "exchanged-fb-token", nil
}

type mockDeliveryServiceClientForOAuth struct {
	deliverypb.DeliveryServiceClient
	createdProfiles []*deliverypb.CreatePartnerProfileRequest
	deletedProfiles []*deliverypb.DeletePartnerProfileRequest
	createErr       error
}

func (m *mockDeliveryServiceClientForOAuth) CreatePartnerProfile(ctx context.Context, in *deliverypb.CreatePartnerProfileRequest, opts ...grpc.CallOption) (*deliverypb.PartnerProfileResponse, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.createdProfiles = append(m.createdProfiles, in)
	return &deliverypb.PartnerProfileResponse{
		Profile: &deliverypb.DeliveryPartnerProfile{
			PartnerId: in.PartnerId,
			Email:     in.Email,
		},
	}, nil
}

func (m *mockDeliveryServiceClientForOAuth) DeletePartnerProfile(ctx context.Context, in *deliverypb.DeletePartnerProfileRequest, opts ...grpc.CallOption) (*deliverypb.DeletePartnerProfileResponse, error) {
	m.deletedProfiles = append(m.deletedProfiles, in)
	return &deliverypb.DeletePartnerProfileResponse{Success: true}, nil
}

func TestFacebookRegister_Customer_Success(t *testing.T) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "test-secret-key-12345", ExpiryMinutes: 15},
	}
	mockUserClient := &mockUserServiceClient{}
	verifier := &mockFacebookVerifier{
		payload: &FacebookTokenPayload{
			UserID:        "fb-user-1",
			Email:         "customer@fb.example.com",
			EmailVerified: true,
			FirstName:     "John",
			LastName:      "Customer",
		},
	}

	svc := NewAuthService(mockRepo, cfg, nil, mockUserClient, verifier)

	resp, err := svc.FacebookRegister(context.Background(), &dto.FacebookAuthRequest{
		AccessToken: "valid-fb-token",
		Role:        "USER",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Role != "USER" && resp.Role != "CUSTOMER" {
		t.Fatalf("expected role USER or CUSTOMER, got %s", resp.Role)
	}
	if resp.CustomerID == "" {
		t.Fatalf("expected non-empty customer_id")
	}

	userCtx, vErr := auth.ValidateToken(resp.AccessToken, cfg.JWT.Secret)
	if vErr != nil {
		t.Fatalf("failed to validate token: %v", vErr)
	}
	if userCtx.CustomerID == "" {
		t.Fatalf("expected customer_id claim in JWT token")
	}

	oauthAcc, oErr := mockRepo.GetOAuthAccount(context.Background(), model.AuthProviderFacebook, "fb-user-1")
	if oErr != nil || oauthAcc == nil {
		t.Fatalf("expected oauth account persisted, got: %v", oErr)
	}
}

func TestFacebookRegister_Merchant_Success(t *testing.T) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "test-secret-key-12345", ExpiryMinutes: 15},
	}
	mockMerchantClient := &mockMerchantClient{}
	verifier := &mockFacebookVerifier{
		payload: &FacebookTokenPayload{
			UserID:        "fb-merchant-1",
			Email:         "merchant@fb.example.com",
			EmailVerified: true,
			FirstName:     "Mary",
			LastName:      "Merchant",
		},
	}

	svc := NewAuthService(mockRepo, cfg, nil, mockMerchantClient, verifier)

	resp, err := svc.FacebookRegister(context.Background(), &dto.FacebookAuthRequest{
		AccessToken: "valid-fb-token",
		Role:        "MERCHANT",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Role != "MERCHANT" {
		t.Fatalf("expected role MERCHANT, got %s", resp.Role)
	}
	if resp.MerchantID == "" {
		t.Fatalf("expected non-empty merchant_id")
	}

	userCtx, vErr := auth.ValidateToken(resp.AccessToken, cfg.JWT.Secret)
	if vErr != nil {
		t.Fatalf("failed to validate token: %v", vErr)
	}
	if userCtx.MerchantID == "" {
		t.Fatalf("expected merchant_id claim in JWT token")
	}
	if len(mockMerchantClient.createdReqs) != 1 {
		t.Fatalf("expected merchant profile created")
	}
}

func TestFacebookRegister_DeliveryPartner_Success(t *testing.T) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "test-secret-key-12345", ExpiryMinutes: 15},
	}
	mockDeliveryClient := &mockDeliveryServiceClientForOAuth{}
	verifier := &mockFacebookVerifier{
		payload: &FacebookTokenPayload{
			UserID:        "fb-partner-1",
			Email:         "partner@fb.example.com",
			EmailVerified: true,
			FirstName:     "Dave",
			LastName:      "Delivery",
		},
	}

	svc := NewAuthService(mockRepo, cfg, nil, mockDeliveryClient, verifier)

	resp, err := svc.FacebookRegister(context.Background(), &dto.FacebookAuthRequest{
		AccessToken: "valid-fb-token",
		Role:        "DELIVERY_PARTNER",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Role != "DELIVERY_PARTNER" {
		t.Fatalf("expected role DELIVERY_PARTNER, got %s", resp.Role)
	}
	if resp.PartnerID == "" {
		t.Fatalf("expected non-empty partner_id")
	}

	userCtx, vErr := auth.ValidateToken(resp.AccessToken, cfg.JWT.Secret)
	if vErr != nil {
		t.Fatalf("failed to validate token: %v", vErr)
	}
	if userCtx.PartnerID == "" {
		t.Fatalf("expected partner_id claim in JWT token")
	}
	if len(mockDeliveryClient.createdProfiles) != 1 {
		t.Fatalf("expected delivery partner profile created")
	}
}

func TestFacebookLogin_UserNotFound_ReturnsNotFoundError(t *testing.T) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret-key-12345"}}
	verifier := &mockFacebookVerifier{
		payload: &FacebookTokenPayload{
			UserID:        "fb-unknown",
			Email:         "unknown@fb.example.com",
			EmailVerified: true,
		},
	}

	svc := NewAuthService(mockRepo, cfg, nil, verifier)

	_, err := svc.FacebookLogin(context.Background(), &dto.FacebookAuthRequest{
		AccessToken: "valid-fb-token",
		Role:        "USER",
	})
	if err == nil {
		t.Fatalf("expected error for unlinked/unregistered user login")
	}
	var appErr *appErrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != appErrors.CodeNotFound {
		t.Fatalf("expected NotFound error, got: %v", err)
	}
}

func TestFacebookRegister_UserAlreadyExists_ReturnsConflict(t *testing.T) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret-key-12345"}}
	verifier := &mockFacebookVerifier{
		payload: &FacebookTokenPayload{
			UserID:        "fb-existing",
			Email:         "existing@fb.example.com",
			EmailVerified: true,
		},
	}

	_ = mockRepo.CreateCredential(context.Background(), &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        uuid.Must(uuid.NewV7()),
		Email:         "existing@fb.example.com",
		Role:          model.RoleUser,
		EmailVerified: true,
		IsActive:      true,
	})

	svc := NewAuthService(mockRepo, cfg, nil, verifier)

	_, err := svc.FacebookRegister(context.Background(), &dto.FacebookAuthRequest{
		AccessToken: "valid-fb-token",
		Role:        "USER",
	})
	if err == nil {
		t.Fatalf("expected conflict error when user already exists")
	}
	var appErr *appErrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != appErrors.CodeConflict {
		t.Fatalf("expected Conflict error, got: %v", err)
	}
}

func TestFacebookLogin_ExistingMapping_AuthenticatesEvenIfEmailChanged(t *testing.T) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret-key-12345"}}

	linkedUserID := uuid.Must(uuid.NewV7())
	_ = mockRepo.CreateCredential(context.Background(), &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        linkedUserID,
		Email:         "old.email@fb.example.com",
		Role:          model.RoleUser,
		EmailVerified: true,
		IsActive:      true,
	})
	_ = mockRepo.CreateOAuthAccount(context.Background(), &model.OAuthAccount{
		ID:             uuid.Must(uuid.NewV7()),
		UserID:         linkedUserID,
		Provider:       model.AuthProviderFacebook,
		ProviderUserID: "fb-mapped-123",
		Email:          "old.email@fb.example.com",
	})

	// Facebook returns new email
	verifier := &mockFacebookVerifier{
		payload: &FacebookTokenPayload{
			UserID:        "fb-mapped-123",
			Email:         "new.changed.email@fb.example.com",
			EmailVerified: true,
			FirstName:     "Jane",
			LastName:      "Doe",
		},
	}

	svc := NewAuthService(mockRepo, cfg, nil, verifier)

	resp, err := svc.FacebookLogin(context.Background(), &dto.FacebookAuthRequest{
		AccessToken: "valid-token",
		Role:        "USER",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.UserID != linkedUserID.String() {
		t.Fatalf("expected linked user ID %s, got %s", linkedUserID.String(), resp.UserID)
	}
}

func TestFacebookLogin_ConflictingIdentityMapping_Rejected(t *testing.T) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret-key-12345"}}

	user1ID := uuid.Must(uuid.NewV7())
	user2ID := uuid.Must(uuid.NewV7())

	_ = mockRepo.CreateCredential(context.Background(), &model.AuthCredential{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        user2ID,
		Email:         "user2@fb.example.com",
		Role:          model.RoleUser,
		EmailVerified: true,
		IsActive:      true,
	})

	// Pre-link provider identity to user1
	mockRepo.oauthAccounts = map[string]*model.OAuthAccount{
		model.AuthProviderFacebook.String() + ":fb-conflict-id": {
			ID:             uuid.Must(uuid.NewV7()),
			UserID:         user1ID,
			Provider:       model.AuthProviderFacebook,
			ProviderUserID: "fb-conflict-id",
			Email:          "user1@fb.example.com",
		},
	}

	verifier := &mockFacebookVerifier{
		payload: &FacebookTokenPayload{
			UserID:        "fb-conflict-id",
			Email:         "user2@fb.example.com",
			EmailVerified: true,
		},
	}

	svc := NewAuthService(mockRepo, cfg, nil, verifier)

	// Attempting to register with provider identity that is already linked to another user must be rejected with CONFLICT
	_, err := svc.FacebookRegister(context.Background(), &dto.FacebookAuthRequest{
		AccessToken: "valid-token",
		Role:        "USER",
	})
	if err == nil {
		t.Fatalf("expected conflict error when registering conflicting identity")
	}
	var appErr *appErrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != appErrors.CodeConflict {
		t.Fatalf("expected Conflict error, got: %v", err)
	}
}

func TestFacebookRegister_UnverifiedEmail_Rejected(t *testing.T) {
	mockRepo := newMockAuthRepository()
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret-key-12345"}}
	verifier := &mockFacebookVerifier{
		payload: &FacebookTokenPayload{
			UserID:        "fb-unverified",
			Email:         "unverified@fb.example.com",
			EmailVerified: false,
		},
	}

	svc := NewAuthService(mockRepo, cfg, nil, verifier)

	_, err := svc.FacebookRegister(context.Background(), &dto.FacebookAuthRequest{
		AccessToken: "valid-token",
		Role:        "USER",
	})
	if err == nil {
		t.Fatalf("expected error for unverified email")
	}
	var appErr *appErrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != appErrors.CodeUnauthorized {
		t.Fatalf("expected Unauthorized error, got: %v", err)
	}
}

func TestFacebookRegister_RollsBackDeliveryPartnerOnCredentialFailure(t *testing.T) {
	mockRepo := newMockAuthRepository()
	mockRepo.createCredWithOAuthErr = errors.New("db failure")
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret-key-12345"}}
	mockDeliveryClient := &mockDeliveryServiceClientForOAuth{}
	verifier := &mockFacebookVerifier{
		payload: &FacebookTokenPayload{
			UserID:        "fb-rollback-partner",
			Email:         "partner.fail@fb.example.com",
			EmailVerified: true,
		},
	}

	svc := NewAuthService(mockRepo, cfg, nil, mockDeliveryClient, verifier)

	_, err := svc.FacebookRegister(context.Background(), &dto.FacebookAuthRequest{
		AccessToken: "valid-token",
		Role:        "DELIVERY_PARTNER",
	})
	if err == nil {
		t.Fatalf("expected error on db failure")
	}

	if len(mockDeliveryClient.createdProfiles) != 1 {
		t.Fatalf("expected 1 delivery profile creation attempt")
	}
	if len(mockDeliveryClient.deletedProfiles) != 1 {
		t.Fatalf("expected 1 compensating delivery profile deletion call, got %d", len(mockDeliveryClient.deletedProfiles))
	}
}
