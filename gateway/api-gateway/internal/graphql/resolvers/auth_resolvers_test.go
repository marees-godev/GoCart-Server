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
	lastRegisterReq    *authpb.RegisterRequest
	lastLoginReq       *authpb.LoginRequest
	lastVerifyEmailReq *authpb.VerifyEmailRequest
	lastResendReq      *authpb.ResendVerificationEmailRequest
}

func (m *mockAuthClient) Register(ctx context.Context, in *authpb.RegisterRequest, opts ...grpcPkg.CallOption) (*authpb.AuthResponse, error) {
	m.lastRegisterReq = in
	partnerID := ""
	fn := in.FirstName
	ln := in.LastName
	switch in.Role {
	case authpb.Role_DELIVERY_PARTNER:
		partnerID = "partner-uuid-123"
	case authpb.Role_MERCHANT:
		fn = "Jane"
		ln = "Merchant"
	}
	return &authpb.AuthResponse{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresIn:    900,
		UserId:       "user-uuid-123",
		Role:         in.Role,
		PartnerId:    partnerID,
		FirstName:    fn,
		LastName:     ln,
	}, nil
}

func (m *mockAuthClient) Login(ctx context.Context, in *authpb.LoginRequest, opts ...grpcPkg.CallOption) (*authpb.AuthResponse, error) {
	m.lastLoginReq = in
	return &authpb.AuthResponse{
		AccessToken:  "test-login-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresIn:    900,
		UserId:       "user-uuid-123",
		Role:         in.Role,
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

func (m *mockAuthClient) VerifyEmail(ctx context.Context, in *authpb.VerifyEmailRequest, opts ...grpcPkg.CallOption) (*authpb.VerifyEmailResponse, error) {
	m.lastVerifyEmailReq = in
	return &authpb.VerifyEmailResponse{
		Success: true,
		Message: "Email verified",
	}, nil
}

func (m *mockAuthClient) ResendVerificationEmail(ctx context.Context, in *authpb.ResendVerificationEmailRequest, opts ...grpcPkg.CallOption) (*authpb.ResendVerificationEmailResponse, error) {
	m.lastResendReq = in
	return &authpb.ResendVerificationEmailResponse{
		Success: true,
		Message: "Verification sent",
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
	input := model.RegisterInput{
		Email:     "customer@example.com",
		Password:  "Password123!",
		Role:      model.AuthRoleUser,
		FirstName: &fn,
		LastName:  &ln,
	}

	payload, err := r.Register(context.Background(), input)
	if err != nil {
		t.Fatalf("expected successful customer registration, got err: %v", err)
	}

	if payload.Token != "test-access-token" {
		t.Errorf("expected token 'test-access-token', got '%s'", payload.Token)
	}

	// Verify AuthClient called with Role == USER
	if authMock.lastRegisterReq == nil {
		t.Fatal("expected AuthClient.Register to be called")
	}
	if authMock.lastRegisterReq.Role != authpb.Role_USER {
		t.Errorf("expected Role to be USER, got %v", authMock.lastRegisterReq.Role)
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

	mFn := "Jane"
	mLn := "Merchant"
	input := model.RegisterInput{
		Email:     "merchant@example.com",
		Password:  "Password123!",
		Role:      model.AuthRoleMerchant,
		FirstName: &mFn,
		LastName:  &mLn,
	}

	payload, err := r.Register(context.Background(), input)
	if err != nil {
		t.Fatalf("expected successful merchant registration, got err: %v", err)
	}

	if payload.Token != "test-access-token" {
		t.Errorf("expected token 'test-access-token', got '%s'", payload.Token)
	}

	// Verify AuthClient called with Role == MERCHANT
	if authMock.lastRegisterReq == nil {
		t.Fatal("expected AuthClient.Register to be called")
	}
	if authMock.lastRegisterReq.Role != authpb.Role_MERCHANT {
		t.Errorf("expected Role to be MERCHANT, got %v", authMock.lastRegisterReq.Role)
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
		Email:    "customer@example.com",
		Password: "Password123!",
		Role:     model.AuthRoleUser,
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
	if authMock.lastLoginReq.Role != authpb.Role_USER {
		t.Errorf("expected Role to be USER, got %v", authMock.lastLoginReq.Role)
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
		Email:    "merchant@example.com",
		Password: "Password123!",
		Role:     model.AuthRoleMerchant,
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
	if authMock.lastLoginReq.Role != authpb.Role_MERCHANT {
		t.Errorf("expected Role to be MERCHANT, got %v", authMock.lastLoginReq.Role)
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
			Role:         authpb.Role_USER,
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
		Email: "user@example.com",
		Role:  model.AuthRoleUser,
	})
	if err != nil {
		t.Fatalf("expected successful forgotPassword, got: %v", err)
	}
	if !resp.Success || !strings.Contains(resp.Message, "password reset code has been sent") {
		t.Errorf("unexpected payload: %+v", resp)
	}

	// 2. Missing email
	_, errMissing := r.ForgotPassword(context.Background(), model.ForgotPasswordInput{
		Email: "",
		Role:  model.AuthRoleUser,
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
		Role:        model.AuthRoleUser,
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
		Role:        model.AuthRoleUser,
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

func TestRegisterResolver_DeliveryPartner_Alone(t *testing.T) {
	authMock := &mockAuthClient{}
	userMock := &mockUserClient{}
	deliveryMock := &mockDeliveryClient{}

	clients := &grpc.Clients{
		AuthClient:     authMock,
		UserClient:     userMock,
		DeliveryClient: deliveryMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	fn := "Fast"
	ln := "Logistics"
	input := model.RegisterInput{
		Email:     "deliverypartner@example.com",
		Password:  "Password123!",
		Role:      model.AuthRoleDeliveryPartner,
		FirstName: &fn,
		LastName:  &ln,
	}

	payload, err := r.Register(context.Background(), input)
	if err != nil {
		t.Fatalf("expected successful delivery partner registration, got err: %v", err)
	}

	if payload.Token != "test-access-token" {
		t.Errorf("expected token 'test-access-token', got '%s'", payload.Token)
	}
	if payload.Role == nil || *payload.Role != model.RoleDeliveryPartner {
		t.Errorf("expected role DELIVERY_PARTNER, got %v", payload.Role)
	}

	if deliveryMock.lastCreatePartnerReq == nil {
		t.Fatalf("expected CreatePartnerProfile to be recorded in DeliveryService")
	}
	if deliveryMock.lastCreatePartnerReq.PartnerId != "user-uuid-123" {
		t.Errorf("expected partnerId user-uuid-123, got %s", deliveryMock.lastCreatePartnerReq.PartnerId)
	}
	if deliveryMock.lastCreatePartnerReq.CompanyName != "" {
		t.Errorf("expected empty companyName upon registration, got %s", deliveryMock.lastCreatePartnerReq.CompanyName)
	}
	if deliveryMock.lastCreatePartnerReq.Email != "deliverypartner@example.com" {
		t.Errorf("expected email deliverypartner@example.com, got %s", deliveryMock.lastCreatePartnerReq.Email)
	}
}

func TestRegisterResolver_DeliveryPartner_DeliveryClientUnavailable(t *testing.T) {
	authMock := &mockAuthClient{}
	userMock := &mockUserClient{}

	// DeliveryClient is nil
	clients := &grpc.Clients{
		AuthClient: authMock,
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	fn := "Fast"
	ln := "Logistics"
	input := model.RegisterInput{
		Email:     "deliverypartner@example.com",
		Password:  "Password123!",
		Role:      model.AuthRoleDeliveryPartner,
		FirstName: &fn,
		LastName:  &ln,
	}

	_, err := r.Register(context.Background(), input)
	if err == nil {
		t.Fatalf("expected error when delivery client is unavailable for delivery partner registration, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeServiceUnavailable {
		t.Errorf("expected ServiceUnavailable error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "delivery service unavailable") {
		t.Errorf("expected 'delivery service unavailable' in error message, got: %s", err.Error())
	}
}

func TestAuthResolvers_ClientUnavailable(t *testing.T) {
	rNilClients := &mutationResolver{
		Resolver: &Resolver{},
	}

	loginInput := model.LoginInput{
		Email:    "test@example.com",
		Password: "Password123!",
		Role:     model.AuthRoleUser,
	}

	_, errLogin := rNilClients.Login(context.Background(), loginInput)
	if errLogin == nil || !strings.Contains(errLogin.Error(), "auth client unavailable") {
		t.Errorf("expected 'auth client unavailable' error on Login with nil clients, got: %v", errLogin)
	}

	fn := "John"
	registerInput := model.RegisterInput{
		Email:     "test@example.com",
		Password:  "Password123!",
		Role:      model.AuthRoleUser,
		FirstName: &fn,
	}

	_, errRegister := rNilClients.Register(context.Background(), registerInput)
	if errRegister == nil || !strings.Contains(errRegister.Error(), "auth client unavailable") {
		t.Errorf("expected 'auth client unavailable' error on Register with nil clients, got: %v", errRegister)
	}

	rNilAuth := &mutationResolver{
		Resolver: &Resolver{
			Clients: &grpc.Clients{},
		},
	}

	_, errLogin2 := rNilAuth.Login(context.Background(), loginInput)
	if errLogin2 == nil || !strings.Contains(errLogin2.Error(), "auth client unavailable") {
		t.Errorf("expected 'auth client unavailable' error on Login with nil AuthClient, got: %v", errLogin2)
	}

	_, errRegister2 := rNilAuth.Register(context.Background(), registerInput)
	if errRegister2 == nil || !strings.Contains(errRegister2.Error(), "auth client unavailable") {
		t.Errorf("expected 'auth client unavailable' error on Register with nil AuthClient, got: %v", errRegister2)
	}

	var rNilResolver *mutationResolver
	_, errLogin3 := rNilResolver.Login(context.Background(), loginInput)
	if errLogin3 == nil || !strings.Contains(errLogin3.Error(), "auth client unavailable") {
		t.Errorf("expected 'auth client unavailable' error on Login with nil resolver, got: %v", errLogin3)
	}

	_, errRegister3 := rNilResolver.Register(context.Background(), registerInput)
	if errRegister3 == nil || !strings.Contains(errRegister3.Error(), "auth client unavailable") {
		t.Errorf("expected 'auth client unavailable' error on Register with nil resolver, got: %v", errRegister3)
	}
}

func TestVerifyEmail_Resolver(t *testing.T) {
	authCli := &mockAuthClient{}
	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: &grpc.Clients{
				AuthClient: authCli,
			},
		},
	}

	success, err := r.VerifyEmail(context.Background(), "user@example.com", "123456", model.AuthRoleMerchant)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !success {
		t.Errorf("expected success to be true")
	}
	if authCli.lastVerifyEmailReq == nil {
		t.Fatal("expected lastVerifyEmailReq to not be nil")
	}
	if authCli.lastVerifyEmailReq.Email != "user@example.com" {
		t.Errorf("expected email 'user@example.com', got '%s'", authCli.lastVerifyEmailReq.Email)
	}
	if authCli.lastVerifyEmailReq.Otp != "123456" {
		t.Errorf("expected otp '123456', got '%s'", authCli.lastVerifyEmailReq.Otp)
	}
	if authCli.lastVerifyEmailReq.Role != authpb.Role_MERCHANT {
		t.Errorf("expected role MERCHANT, got %v", authCli.lastVerifyEmailReq.Role)
	}
}

func TestResendVerificationEmail_Resolver(t *testing.T) {
	authCli := &mockAuthClient{}
	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: &grpc.Clients{
				AuthClient: authCli,
			},
		},
	}

	success, err := r.ResendVerificationEmail(context.Background(), "delivery@example.com", model.AuthRoleDeliveryPartner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !success {
		t.Errorf("expected success to be true")
	}
	if authCli.lastResendReq == nil {
		t.Fatal("expected lastResendReq to not be nil")
	}
	if authCli.lastResendReq.Email != "delivery@example.com" {
		t.Errorf("expected email 'delivery@example.com', got '%s'", authCli.lastResendReq.Email)
	}
	if authCli.lastResendReq.Role != authpb.Role_DELIVERY_PARTNER {
		t.Errorf("expected role DELIVERY_PARTNER, got %v", authCli.lastResendReq.Role)
	}
}

