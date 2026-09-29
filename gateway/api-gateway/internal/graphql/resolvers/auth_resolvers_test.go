package resolvers

import (
	"context"
	"strings"
	"testing"

	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	grpcPkg "google.golang.org/grpc"
)

type mockAuthClient struct {
	authpb.AuthServiceClient
	lastRegisterReq *authpb.RegisterRequest
	lastLoginReq    *authpb.LoginRequest
}

func (m *mockAuthClient) Register(ctx context.Context, in *authpb.RegisterRequest, opts ...grpcPkg.CallOption) (*authpb.AuthResponse, error) {
	m.lastRegisterReq = in
	return &authpb.AuthResponse{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresIn:    900,
		UserId:       "user-uuid-123",
	}, nil
}

func (m *mockAuthClient) Login(ctx context.Context, in *authpb.LoginRequest, opts ...grpcPkg.CallOption) (*authpb.AuthResponse, error) {
	m.lastLoginReq = in
	role := "CUSTOMER"
	if in.IsMerchant {
		role = "MERCHANT"
	}
	return &authpb.AuthResponse{
		AccessToken:  "test-login-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresIn:    900,
		UserId:       "user-uuid-123",
		Role:         role,
	}, nil
}

func (m *mockAuthClient) Logout(ctx context.Context, in *authpb.LogoutRequest, opts ...grpcPkg.CallOption) (*authpb.LogoutResponse, error) {
	if in.AccessToken == "invalid-token" {
		return nil, appErrors.MapAppErrorToGRPC(appErrors.Unauthorized("invalid access token"))
	}
	return &authpb.LogoutResponse{
		Success: true,
		Message: "Logged out successfully",
	}, nil
}

type mockUserClient struct {
	userpb.UserServiceClient
	lastGetUserReq *userpb.GetUserRequest
}

func (m *mockUserClient) GetUser(ctx context.Context, in *userpb.GetUserRequest, opts ...grpcPkg.CallOption) (*userpb.GetUserResponse, error) {
	m.lastGetUserReq = in
	return &userpb.GetUserResponse{
		User: &userpb.User{
			Id:        in.Id,
			Email:     "customer@example.com",
			FirstName: "John",
			LastName:  "Doe",
			CreatedAt: "2026-09-23T10:00:00Z",
		},
	}, nil
}

func TestRegisterResolver_Customer(t *testing.T) {
	authMock := &mockAuthClient{}
	userMock := &mockUserClient{}

	clients := &grpc.Clients{
		AuthClient: authMock,
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	fn := "John"
	ln := "Doe"
	isMerch := false
	input := model.RegisterInput{
		Email:      "customer@example.com",
		Password:   "Password123!",
		FirstName:  &fn,
		LastName:   &ln,
		IsMerchant: isMerch,
	}

	payload, err := r.Register(context.Background(), input)
	if err != nil {
		t.Fatalf("expected successful customer registration, got err: %v", err)
	}

	if payload.Token != "test-access-token" {
		t.Errorf("expected token 'test-access-token', got '%s'", payload.Token)
	}

	// Verify AuthClient called with IsMerchant == false
	if authMock.lastRegisterReq == nil {
		t.Fatal("expected AuthClient.Register to be called")
	}
	if authMock.lastRegisterReq.IsMerchant != false {
		t.Errorf("expected IsMerchant to be false, got true")
	}

	// Verify UserClient.GetUser was called
	if userMock.lastGetUserReq == nil || userMock.lastGetUserReq.Id != "user-uuid-123" {
		t.Errorf("expected UserClient.GetUser to be called with user-uuid-123")
	}

	// Verify payload.User is populated
	if payload.User == nil || payload.User.ID != "user-uuid-123" {
		t.Errorf("expected payload user with ID 'user-uuid-123', got %+v", payload.User)
	}
	if payload.User.CreatedAt == nil || *payload.User.CreatedAt != "2026-09-23T10:00:00Z" {
		t.Errorf("expected payload user with CreatedAt '2026-09-23T10:00:00Z', got %+v", payload.User)
	}
}

func TestRegisterResolver_Merchant(t *testing.T) {
	authMock := &mockAuthClient{}
	userMock := &mockUserClient{}

	clients := &grpc.Clients{
		AuthClient: authMock,
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	fn := "Jane"
	ln := "Merchant"
	isMerch := true
	input := model.RegisterInput{
		Email:      "merchant@example.com",
		Password:   "Password123!",
		FirstName:  &fn,
		LastName:   &ln,
		IsMerchant: isMerch,
	}

	payload, err := r.Register(context.Background(), input)
	if err != nil {
		t.Fatalf("expected successful merchant registration, got err: %v", err)
	}

	if payload.Token != "test-access-token" {
		t.Errorf("expected token 'test-access-token', got '%s'", payload.Token)
	}

	// Verify AuthClient called with IsMerchant == true
	if authMock.lastRegisterReq == nil {
		t.Fatal("expected AuthClient.Register to be called")
	}
	if authMock.lastRegisterReq.IsMerchant != true {
		t.Errorf("expected IsMerchant to be true, got false")
	}

	// Verify UserClient.GetUser was NOT called for merchant
	if userMock.lastGetUserReq != nil {
		t.Errorf("expected UserClient.GetUser not to be called for merchant")
	}

	// Verify payload.User is populated and not null
	if payload.User == nil {
		t.Fatal("expected non-null user in AuthPayload")
	}
	if payload.User.ID != "user-uuid-123" {
		t.Errorf("expected user ID 'user-uuid-123', got '%s'", payload.User.ID)
	}
	if payload.User.Email != "merchant@example.com" {
		t.Errorf("expected email 'merchant@example.com', got '%s'", payload.User.Email)
	}
	if payload.User.FirstName == nil || *payload.User.FirstName != "Jane" || payload.User.LastName == nil || *payload.User.LastName != "Merchant" {
		t.Errorf("expected name Jane Merchant, got %v %v", payload.User.FirstName, payload.User.LastName)
	}
}

func TestLoginResolver_Customer(t *testing.T) {
	authMock := &mockAuthClient{}
	userMock := &mockUserClient{}

	clients := &grpc.Clients{
		AuthClient: authMock,
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	input := model.LoginInput{
		Email:      "customer@example.com",
		Password:   "Password123!",
		IsMerchant: false,
	}

	payload, err := r.Login(context.Background(), input)
	if err != nil {
		t.Fatalf("expected successful customer login, got: %v", err)
	}

	if payload.Token != "test-login-token" {
		t.Errorf("expected token 'test-login-token', got '%s'", payload.Token)
	}
	if payload.RefreshToken == nil || *payload.RefreshToken != "test-refresh-token" {
		t.Errorf("expected refresh token 'test-refresh-token', got %v", payload.RefreshToken)
	}
	if authMock.lastLoginReq == nil {
		t.Fatal("expected AuthClient.Login to be called")
	}
	if authMock.lastLoginReq.IsMerchant != false {
		t.Errorf("expected IsMerchant to be false, got true")
	}
	if userMock.lastGetUserReq == nil || userMock.lastGetUserReq.Id != "user-uuid-123" {
		t.Errorf("expected UserClient.GetUser to be called for customer")
	}
	if payload.Role == nil || *payload.Role != "CUSTOMER" {
		t.Errorf("expected role CUSTOMER, got %v", payload.Role)
	}
}

func TestLoginResolver_Merchant(t *testing.T) {
	authMock := &mockAuthClient{}
	userMock := &mockUserClient{}

	clients := &grpc.Clients{
		AuthClient: authMock,
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	input := model.LoginInput{
		Email:      "merchant@example.com",
		Password:   "Password123!",
		IsMerchant: true,
	}

	payload, err := r.Login(context.Background(), input)
	if err != nil {
		t.Fatalf("expected successful merchant login, got: %v", err)
	}

	if payload.Token != "test-login-token" {
		t.Errorf("expected token 'test-login-token', got '%s'", payload.Token)
	}
	if authMock.lastLoginReq == nil {
		t.Fatal("expected AuthClient.Login to be called")
	}
	if authMock.lastLoginReq.IsMerchant != true {
		t.Errorf("expected IsMerchant to be true, got false")
	}
	if userMock.lastGetUserReq != nil {
		t.Errorf("expected UserClient.GetUser NOT to be called for merchant")
	}
	if payload.Role == nil || *payload.Role != "MERCHANT" {
		t.Errorf("expected role MERCHANT, got %v", payload.Role)
	}
	if payload.Merchant == nil {
		t.Errorf("expected Merchant payload to be populated")
	}
}

func (m *mockAuthClient) RefreshToken(ctx context.Context, in *authpb.RefreshTokenRequest, opts ...grpcPkg.CallOption) (*authpb.AuthResponse, error) {
	if in.GetRefreshToken() == "valid-refresh-token" {
		return &authpb.AuthResponse{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			TokenType:    "Bearer",
			ExpiresIn:    900,
			UserId:       "user-123",
			Role:         "CUSTOMER",
		}, nil
	}
	return nil, appErrors.MapAppErrorToGRPC(appErrors.Unauthorized("invalid refresh token"))
}

func TestRefreshTokenResolver(t *testing.T) {
	authMock := &mockAuthClient{}
	clients := &grpc.Clients{
		AuthClient: authMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	payload, err := r.RefreshToken(context.Background(), model.RefreshTokenInput{
		RefreshToken: "valid-refresh-token",
	})
	if err != nil {
		t.Fatalf("expected successful refresh, got: %v", err)
	}

	if payload.Token != "new-access-token" {
		t.Errorf("expected token 'new-access-token', got '%s'", payload.Token)
	}
	if payload.RefreshToken == nil || *payload.RefreshToken != "new-refresh-token" {
		t.Errorf("expected refresh token 'new-refresh-token', got %v", payload.RefreshToken)
	}

	_, errInvalid := r.RefreshToken(context.Background(), model.RefreshTokenInput{
		RefreshToken: "invalid-token",
	})
	if errInvalid == nil {
		t.Errorf("expected error for invalid refresh token, got nil")
	}
}

func TestLogoutResolver(t *testing.T) {
	authMock := &mockAuthClient{}
	clients := &grpc.Clients{
		AuthClient: authMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctxWithAuth := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Email:  "user123@example.com",
		Role:   "CUSTOMER",
	})

	// 1. Authenticated logout
	payload, err := r.Logout(ctxWithAuth)
	if err != nil {
		t.Fatalf("expected successful logout, got: %v", err)
	}

	if !payload.Success {
		t.Errorf("expected payload success true, got false")
	}
	if payload.Message == nil || *payload.Message != "Logged out successfully" {
		t.Errorf("expected message 'Logged out successfully', got %v", payload.Message)
	}

	// 2. Unauthenticated logout should fail
	_, errUnauth := r.Logout(context.Background())
	if errUnauth == nil {
		t.Errorf("expected error for unauthenticated logout, got nil")
	}
}

func (m *mockAuthClient) ForgotPassword(ctx context.Context, in *authpb.ForgotPasswordRequest, opts ...grpcPkg.CallOption) (*authpb.ForgotPasswordResponse, error) {
	if in.GetEmail() == "" {
		return nil, appErrors.MapAppErrorToGRPC(appErrors.BadRequest("email is required"))
	}
	return &authpb.ForgotPasswordResponse{
		Success: true,
		Message: "If an account exists with this email address, a password reset code has been sent.",
	}, nil
}

func (m *mockAuthClient) ResetPasswordWithOtp(ctx context.Context, in *authpb.ResetPasswordWithOtpRequest, opts ...grpcPkg.CallOption) (*authpb.ResetPasswordResponse, error) {
	if in.GetOtp() == "invalid-otp" {
		return nil, appErrors.MapAppErrorToGRPC(appErrors.BadRequest("invalid password reset code"))
	}
	return &authpb.ResetPasswordResponse{
		Success: true,
		Message: "Password has been reset successfully",
	}, nil
}

func (m *mockAuthClient) ChangePassword(ctx context.Context, in *authpb.ChangePasswordRequest, opts ...grpcPkg.CallOption) (*authpb.ChangePasswordResponse, error) {
	if in.GetOldPassword() == "wrong-old-password" {
		return nil, appErrors.MapAppErrorToGRPC(appErrors.InvalidCredentials("invalid current password"))
	}
	return &authpb.ChangePasswordResponse{
		Success: true,
		Message: "Password changed successfully",
	}, nil
}

func TestForgotPasswordResolver(t *testing.T) {
	authMock := &mockAuthClient{}
	clients := &grpc.Clients{
		AuthClient: authMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	// 1. Success
	resp, err := r.ForgotPassword(context.Background(), model.ForgotPasswordInput{
		Email:      "user@example.com",
		IsMerchant: false,
	})
	if err != nil {
		t.Fatalf("expected successful forgotPassword, got: %v", err)
	}
	if !resp.Success || !strings.Contains(resp.Message, "password reset code has been sent") {
		t.Errorf("unexpected payload: %+v", resp)
	}

	// 2. Missing email
	_, errMissing := r.ForgotPassword(context.Background(), model.ForgotPasswordInput{
		Email:      "",
		IsMerchant: false,
	})
	if errMissing == nil {
		t.Errorf("expected error for empty email, got nil")
	}
}

func TestResetPasswordWithOtpResolver(t *testing.T) {
	authMock := &mockAuthClient{}
	clients := &grpc.Clients{
		AuthClient: authMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	// 1. Success
	resp, err := r.ResetPasswordWithOtp(context.Background(), model.ResetPasswordWithOtpInput{
		Email:       "user@example.com",
		Otp:         "123456",
		NewPassword: "NewPassword123!",
		IsMerchant:  false,
	})
	if err != nil {
		t.Fatalf("expected successful resetPasswordWithOtp, got: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success true, got false")
	}

	// 2. Invalid OTP
	_, errInvalid := r.ResetPasswordWithOtp(context.Background(), model.ResetPasswordWithOtpInput{
		Email:       "user@example.com",
		Otp:         "invalid-otp",
		NewPassword: "NewPassword123!",
		IsMerchant:  false,
	})
	if errInvalid == nil {
		t.Errorf("expected error for invalid otp, got nil")
	}
}

func TestChangePasswordResolver(t *testing.T) {
	authMock := &mockAuthClient{}
	clients := &grpc.Clients{
		AuthClient: authMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctxWithAuth := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Email:  "user123@example.com",
		Role:   "CUSTOMER",
	})

	// 1. Unauthenticated changePassword must fail
	_, errUnauth := r.ChangePassword(context.Background(), model.ChangePasswordInput{
		OldPassword: "OldPassword123!",
		NewPassword: "NewPassword123!",
	})
	if errUnauth == nil {
		t.Fatalf("expected unauthenticated changePassword to fail")
	}

	// 2. Incorrect old password -> rejected with INVALID_CREDENTIALS
	_, errWrong := r.ChangePassword(ctxWithAuth, model.ChangePasswordInput{
		OldPassword: "wrong-old-password",
		NewPassword: "NewPassword123!",
	})
	if errWrong == nil {
		t.Fatalf("expected incorrect old password to fail")
	}
	appErr, ok := errWrong.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeInvalidCredentials {
		t.Errorf("expected CodeInvalidCredentials, got: %v", errWrong)
	}

	// 3. Success
	resp, err := r.ChangePassword(ctxWithAuth, model.ChangePasswordInput{
		OldPassword: "CorrectOldPassword123!",
		NewPassword: "NewValidPassword123!",
	})
	if err != nil {
		t.Fatalf("expected changePassword to succeed, got: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success true, got false")
	}
}
