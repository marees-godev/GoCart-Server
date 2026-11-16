package resolvers

import (
	"bytes"
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	grpcPkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockUserClientForUserResolvers struct {
	userpb.UserServiceClient
	lastReactivateReq *userpb.ReactivateUserRequest
	lastUpdateReq     *userpb.UpdateUserRequest
	getUserResp       *userpb.GetUserResponse
	reactivateErr     error
	updateErr         error
	getUserErr        error
}

func (m *mockUserClientForUserResolvers) GetUser(ctx context.Context, in *userpb.GetUserRequest, opts ...grpcPkg.CallOption) (*userpb.GetUserResponse, error) {
	if m.getUserErr != nil {
		return nil, m.getUserErr
	}
	if m.getUserResp != nil {
		return m.getUserResp, nil
	}
	return nil, status.Error(codes.NotFound, "user not found")
}

func (m *mockUserClientForUserResolvers) ReactivateUser(ctx context.Context, in *userpb.ReactivateUserRequest, opts ...grpcPkg.CallOption) (*userpb.ReactivateUserResponse, error) {
	m.lastReactivateReq = in
	if m.reactivateErr != nil {
		return nil, m.reactivateErr
	}
	return &userpb.ReactivateUserResponse{
		Success: true,
		Message: "account reactivated successfully",
		Status:  "active",
	}, nil
}

func (m *mockUserClientForUserResolvers) UpdateUser(ctx context.Context, in *userpb.UpdateUserRequest, opts ...grpcPkg.CallOption) (*userpb.UpdateUserResponse, error) {
	m.lastUpdateReq = in
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	return &userpb.UpdateUserResponse{
		User: &userpb.User{
			Id:        in.Id,
			AvatarUrl: in.AvatarUrl,
		},
	}, nil
}

func TestReactivateAccount_Success(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "CUSTOMER",
	})

	res, err := r.ReactivateAccount(ctx)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !res.Success || res.Status != "active" {
		t.Errorf("expected active status and success=true, got %+v", res)
	}

	if userMock.lastReactivateReq == nil || userMock.lastReactivateReq.Id != "user-123" {
		t.Errorf("expected reactivate request with id 'user-123', got %+v", userMock.lastReactivateReq)
	}
}

func TestReactivateAccount_Unauthenticated(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := context.Background()

	_, err := r.ReactivateAccount(ctx)
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeUnauthorized {
		t.Errorf("expected UNAUTHORIZED code, got %s", appErr.Code)
	}
}

func TestReactivateAccount_PermissionDenied_MapsToForbidden(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{
		reactivateErr: status.Error(codes.PermissionDenied, "account deactivation period of 30 days has expired; account has been permanently deleted"),
	}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "CUSTOMER",
	})

	_, err := r.ReactivateAccount(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr == nil {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
	if appErr.HTTPStatus != 403 {
		t.Errorf("expected 403 HTTP status, got %d", appErr.HTTPStatus)
	}
}

func TestUpdateUser_InvalidArgument_MapsToBadRequest(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{
		updateErr: status.Error(codes.InvalidArgument, "gender must be one of: male, female, others"),
	}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "CUSTOMER",
	})

	_, err := r.UpdateUser(ctx, "user-123", model.UpdateUserInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr == nil {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST code, got %s", appErr.Code)
	}
	if appErr.HTTPStatus != 400 {
		t.Errorf("expected 400 HTTP status, got %d", appErr.HTTPStatus)
	}
}

func TestUpdateUser_AvatarUpload_ForwardsToGRPC(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "CUSTOMER",
	})

	uploadData := []byte("fake-png-image-content")
	input := model.UpdateUserInput{
		Avatar: &graphql.Upload{
			File:        bytes.NewReader(uploadData),
			ContentType: "image/png",
			Filename:    "my-avatar.png",
			Size:        int64(len(uploadData)),
		},
	}

	res, err := r.UpdateUser(ctx, "user-123", input)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if res.ID != "user-123" {
		t.Errorf("expected user id 'user-123', got '%s'", res.ID)
	}

	if userMock.lastUpdateReq == nil {
		t.Fatal("expected gRPC UpdateUser to be called, but lastUpdateReq is nil")
	}

	if !bytes.Equal(userMock.lastUpdateReq.AvatarImage, uploadData) {
		t.Errorf("expected AvatarImage bytes %v, got %v", uploadData, userMock.lastUpdateReq.AvatarImage)
	}

	if userMock.lastUpdateReq.AvatarContentType == nil || *userMock.lastUpdateReq.AvatarContentType != "image/png" {
		t.Errorf("expected AvatarContentType 'image/png', got %v", userMock.lastUpdateReq.AvatarContentType)
	}

	if userMock.lastUpdateReq.AvatarFilename == nil || *userMock.lastUpdateReq.AvatarFilename != "my-avatar.png" {
		t.Errorf("expected AvatarFilename 'my-avatar.png', got %v", userMock.lastUpdateReq.AvatarFilename)
	}
}

type mockMerchantClientForUserResolvers struct {
	merchantpb.MerchantServiceClient
	getMerchantResp *merchantpb.GetMerchantResponse
	getMerchantErr  error
}

func (m *mockMerchantClientForUserResolvers) GetMerchant(ctx context.Context, in *merchantpb.GetMerchantRequest, opts ...grpcPkg.CallOption) (*merchantpb.GetMerchantResponse, error) {
	if m.getMerchantErr != nil {
		return nil, m.getMerchantErr
	}
	if m.getMerchantResp != nil {
		return m.getMerchantResp, nil
	}
	return nil, status.Error(codes.NotFound, "merchant not found")
}

type mockDeliveryClientForUserResolvers struct {
	deliverypb.DeliveryServiceClient
	getProfileResp *deliverypb.PartnerProfileResponse
	getProfileErr  error
}

func (m *mockDeliveryClientForUserResolvers) GetPartnerProfile(ctx context.Context, in *deliverypb.GetPartnerProfileRequest, opts ...grpcPkg.CallOption) (*deliverypb.PartnerProfileResponse, error) {
	if m.getProfileErr != nil {
		return nil, m.getProfileErr
	}
	if m.getProfileResp != nil {
		return m.getProfileResp, nil
	}
	return nil, status.Error(codes.NotFound, "partner not found")
}

type mockAuthClientForUserResolvers struct {
	authpb.AuthServiceClient
	validateTokenResp *authpb.ValidateTokenResponse
	validateTokenErr  error
}

func (m *mockAuthClientForUserResolvers) ValidateToken(ctx context.Context, in *authpb.ValidateTokenRequest, opts ...grpcPkg.CallOption) (*authpb.ValidateTokenResponse, error) {
	if m.validateTokenErr != nil {
		return nil, m.validateTokenErr
	}
	if m.validateTokenResp != nil {
		return m.validateTokenResp, nil
	}
	return &authpb.ValidateTokenResponse{Valid: false}, nil
}

func TestMe_CustomerRole_Success(t *testing.T) {
	firstName := "John"
	lastName := "Customer"
	userMock := &mockUserClientForUserResolvers{
		getUserResp: &userpb.GetUserResponse{
			User: &userpb.User{
				Id:        "user-cust-1",
				Email:     "customer@example.com",
				FirstName: firstName,
				LastName:  lastName,
				Status:    "active",
			},
		},
	}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &queryResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-cust-1",
		Role:   "CUSTOMER",
		Email:  "customer@example.com",
	})

	me, err := r.Me(ctx)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if me.ID != "user-cust-1" {
		t.Errorf("expected user ID 'user-cust-1', got %s", me.ID)
	}
	if me.Email != "customer@example.com" {
		t.Errorf("expected email 'customer@example.com', got %s", me.Email)
	}
	if me.Role == nil || *me.Role != "CUSTOMER" {
		t.Errorf("expected role 'CUSTOMER', got %v", me.Role)
	}
	if me.FirstName == nil || *me.FirstName != "John" {
		t.Errorf("expected first name 'John', got %v", me.FirstName)
	}
}

func TestMe_MerchantRole_Success(t *testing.T) {
	merchMock := &mockMerchantClientForUserResolvers{
		getMerchantResp: &merchantpb.GetMerchantResponse{
			Merchant: &merchantpb.MerchantResponseData{
				Id:            "merch-123",
				BusinessEmail: "merchant@example.com",
				FirstName:     "Alice",
				LastName:      "Merchant",
				BusinessPhone: "+1234567890",
				Status:        merchantpb.MerchantStatus_APPROVED,
				CreatedAt:     timestamppb.Now(),
				UpdatedAt:     timestamppb.Now(),
			},
		},
	}
	userMock := &mockUserClientForUserResolvers{}
	clients := &grpc.Clients{
		UserClient:     userMock,
		MerchantClient: merchMock,
	}

	r := &queryResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:     "merch-123",
		MerchantID: "merch-123",
		Role:       "MERCHANT",
		Email:      "merchant@example.com",
	})

	me, err := r.Me(ctx)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if me.ID != "merch-123" {
		t.Errorf("expected ID 'merch-123', got %s", me.ID)
	}
	if me.Email != "merchant@example.com" {
		t.Errorf("expected email 'merchant@example.com', got %s", me.Email)
	}
	if me.Role == nil || *me.Role != "MERCHANT" {
		t.Errorf("expected role 'MERCHANT', got %v", me.Role)
	}
	if me.FirstName == nil || *me.FirstName != "Alice" {
		t.Errorf("expected first name 'Alice', got %v", me.FirstName)
	}
	if me.Phone == nil || *me.Phone != "+1234567890" {
		t.Errorf("expected phone '+1234567890', got %v", me.Phone)
	}
}

func TestMe_DeliveryPartnerRole_Success(t *testing.T) {
	phone := "+9876543210"
	deliveryMock := &mockDeliveryClientForUserResolvers{
		getProfileResp: &deliverypb.PartnerProfileResponse{
			Profile: &deliverypb.DeliveryPartnerProfile{
				Id:             "partner-prof-1",
				PartnerId:      "partner-123",
				CompanyName:    "Fast Express",
				Email:          "partner@example.com",
				ContactNumber:  phone,
				ApprovalStatus: "APPROVED",
				IsActive:       true,
				CreatedAt:      "2026-01-01T00:00:00Z",
			},
		},
	}
	userMock := &mockUserClientForUserResolvers{}
	clients := &grpc.Clients{
		UserClient:     userMock,
		DeliveryClient: deliveryMock,
	}

	r := &queryResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:    "partner-123",
		PartnerID: "partner-123",
		Role:      "DELIVERY_PARTNER",
		Email:     "partner@example.com",
	})

	me, err := r.Me(ctx)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if me.ID != "partner-123" {
		t.Errorf("expected ID 'partner-123', got %s", me.ID)
	}
	if me.Email != "partner@example.com" {
		t.Errorf("expected email 'partner@example.com', got %s", me.Email)
	}
	if me.Role == nil || *me.Role != "DELIVERY_PARTNER" {
		t.Errorf("expected role 'DELIVERY_PARTNER', got %v", me.Role)
	}
	if me.Phone == nil || *me.Phone != phone {
		t.Errorf("expected phone '%s', got %v", phone, me.Phone)
	}
	if me.Username == nil || *me.Username != "Fast Express" {
		t.Errorf("expected username 'Fast Express', got %v", me.Username)
	}
}

func TestMe_AdminRole_Success(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &queryResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-system-id",
		Role:   "ADMIN",
		Email:  "admin@example.com",
	})

	me, err := r.Me(ctx)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if me.ID != "admin-system-id" {
		t.Errorf("expected ID 'admin-system-id', got %s", me.ID)
	}
	if me.Email != "admin@example.com" {
		t.Errorf("expected email 'admin@example.com', got %s", me.Email)
	}
	if me.Role == nil || *me.Role != "ADMIN" {
		t.Errorf("expected role 'ADMIN', got %v", me.Role)
	}
	if me.Status == nil || *me.Status != "ACTIVE" {
		t.Errorf("expected status 'ACTIVE', got %v", me.Status)
	}
}

func TestMe_AdminRole_FromUserClient_Success(t *testing.T) {
	adminFirst := "Super"
	adminLast := "Admin"
	userMock := &mockUserClientForUserResolvers{
		getUserResp: &userpb.GetUserResponse{
			User: &userpb.User{
				Id:        "admin-user-1",
				Email:     "superadmin@example.com",
				FirstName: adminFirst,
				LastName:  adminLast,
				Status:    "active",
			},
		},
	}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &queryResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-user-1",
		Role:   "ADMIN",
		Email:  "superadmin@example.com",
	})

	me, err := r.Me(ctx)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if me.ID != "admin-user-1" {
		t.Errorf("expected ID 'admin-user-1', got %s", me.ID)
	}
	if me.FirstName == nil || *me.FirstName != adminFirst {
		t.Errorf("expected first name '%s', got %v", adminFirst, me.FirstName)
	}
	if me.Role == nil || *me.Role != "ADMIN" {
		t.Errorf("expected role 'ADMIN', got %v", me.Role)
	}
}

func TestMe_Unauthenticated(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &queryResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := context.Background()

	_, err := r.Me(ctx)
	if err == nil {
		t.Fatal("expected error for unauthenticated call, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeUnauthorized {
		t.Errorf("expected UNAUTHORIZED code, got %v", err)
	}
}

func TestMe_NonExistent_Customer_ReturnsNotFound(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{
		getUserErr: status.Error(codes.NotFound, "user not found"),
	}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &queryResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "non-existent-user",
		Role:   "CUSTOMER",
	})

	_, err := r.Me(ctx)
	if err == nil {
		t.Fatal("expected error for nonexistent user, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeNotFound {
		t.Errorf("expected NOT_FOUND code, got %v", err)
	}
}

func TestMe_NonExistent_Merchant_ReturnsNotFound(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{
		getUserErr: status.Error(codes.NotFound, "user not found"),
	}
	merchMock := &mockMerchantClientForUserResolvers{
		getMerchantErr: status.Error(codes.NotFound, "merchant not found"),
	}
	clients := &grpc.Clients{
		UserClient:     userMock,
		MerchantClient: merchMock,
	}

	r := &queryResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "non-existent-merchant",
		Role:   "MERCHANT",
	})

	_, err := r.Me(ctx)
	if err == nil {
		t.Fatal("expected error for nonexistent merchant, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeNotFound {
		t.Errorf("expected NOT_FOUND code, got %v", err)
	}
}

func TestMe_NonExistent_DeliveryPartner_ReturnsNotFound(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{
		getUserErr: status.Error(codes.NotFound, "user not found"),
	}
	deliveryMock := &mockDeliveryClientForUserResolvers{
		getProfileErr: status.Error(codes.NotFound, "partner not found"),
	}
	clients := &grpc.Clients{
		UserClient:     userMock,
		DeliveryClient: deliveryMock,
	}

	r := &queryResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "non-existent-partner",
		Role:   "DELIVERY_PARTNER",
	})

	_, err := r.Me(ctx)
	if err == nil {
		t.Fatal("expected error for nonexistent delivery partner, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeNotFound {
		t.Errorf("expected NOT_FOUND code, got %v", err)
	}
}

func TestMe_Admin_InvalidToken_ReturnsNotFound(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{
		getUserErr: status.Error(codes.NotFound, "user not found"),
	}
	authMock := &mockAuthClientForUserResolvers{
		validateTokenResp: &authpb.ValidateTokenResponse{Valid: false},
	}
	clients := &grpc.Clients{
		UserClient: userMock,
		AuthClient: authMock,
	}

	r := &queryResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:   "admin-123",
		Role:     "ADMIN",
		RawToken: "invalid-or-revoked-token",
	})

	_, err := r.Me(ctx)
	if err == nil {
		t.Fatal("expected error for nonexistent/invalid admin, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeNotFound {
		t.Errorf("expected NOT_FOUND code, got %v", err)
	}
}

