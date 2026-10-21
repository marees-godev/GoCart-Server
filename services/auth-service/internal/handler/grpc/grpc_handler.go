package grpc

import (
	"context"
	"log/slog"
	"strings"

	pb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/service"
)

type AuthGRPCHandler struct {
	pb.UnimplementedAuthServiceServer
	authService service.AuthService
	logger      *slog.Logger
}

func NewAuthGRPCHandler(authService service.AuthService, log *slog.Logger) *AuthGRPCHandler {
	if log == nil {
		log = slog.Default()
	}
	return &AuthGRPCHandler{
		authService: authService,
		logger:      log,
	}
}

func (h *AuthGRPCHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.AuthResponse, error) {
	resp, err := h.authService.Login(ctx, &dto.LoginRequest{
		Email:    req.GetEmail(),
		Password: req.GetPassword(),
		Role:     fromPBRole(req.GetRole()),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &pb.AuthResponse{
		AccessToken:   resp.AccessToken,
		RefreshToken:  resp.RefreshToken,
		TokenType:     resp.TokenType,
		ExpiresIn:     int64(resp.ExpiresIn),
		UserId:        resp.UserID,
		Role:          toPBRole(resp.Role),
		MerchantId:    resp.MerchantID,
		BusinessEmail: resp.BusinessEmail,
		FirstName:     resp.FirstName,
		LastName:      resp.LastName,
		PartnerId:     resp.PartnerID,
		CustomerId:    resp.CustomerID,
	}, nil
}

func (h *AuthGRPCHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.AuthResponse, error) {
	resp, err := h.authService.Register(ctx, &dto.RegisterRequest{
		Email:     req.GetEmail(),
		Password:  req.GetPassword(),
		FirstName: req.GetFirstName(),
		LastName:  req.GetLastName(),
		Role:      fromPBRole(req.GetRole()),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &pb.AuthResponse{
		AccessToken:   resp.AccessToken,
		RefreshToken:  resp.RefreshToken,
		TokenType:     resp.TokenType,
		ExpiresIn:     int64(resp.ExpiresIn),
		UserId:        resp.UserID,
		Role:          toPBRole(resp.Role),
		MerchantId:    resp.MerchantID,
		BusinessEmail: resp.BusinessEmail,
		FirstName:     resp.FirstName,
		LastName:      resp.LastName,
		PartnerId:     resp.PartnerID,
		CustomerId:    resp.CustomerID,
	}, nil
}

func (h *AuthGRPCHandler) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	resp, err := h.authService.ValidateToken(ctx, &dto.ValidateTokenRequest{
		Token: req.GetToken(),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &pb.ValidateTokenResponse{
		Valid:  resp.Valid,
		UserId: resp.UserID,
		Email:  resp.Email,
		Role:   toPBRole(resp.Role),
	}, nil
}

func (h *AuthGRPCHandler) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.AuthResponse, error) {
	resp, err := h.authService.RefreshToken(ctx, &dto.RefreshTokenRequest{
		RefreshToken: req.GetRefreshToken(),
		AccessToken:  req.GetAccessToken(),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &pb.AuthResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		TokenType:    resp.TokenType,
		ExpiresIn:    int64(resp.ExpiresIn),
		UserId:       resp.UserID,
		Role:         toPBRole(resp.Role),
		MerchantId:   resp.MerchantID,
		PartnerId:    resp.PartnerID,
		CustomerId:   resp.CustomerID,
	}, nil
}

func (h *AuthGRPCHandler) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	resp, err := h.authService.Logout(ctx, &dto.LogoutRequest{
		AccessToken: req.GetAccessToken(),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &pb.LogoutResponse{
		Success: resp.Success,
		Message: resp.Message,
	}, nil
}

func (h *AuthGRPCHandler) VerifyEmail(ctx context.Context, req *pb.VerifyEmailRequest) (*pb.VerifyEmailResponse, error) {
	resp, err := h.authService.VerifyEmail(ctx, &dto.VerifyEmailRequest{
		Email: req.GetEmail(),
		OTP:   req.GetOtp(),
		Token: req.GetToken(),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &pb.VerifyEmailResponse{
		Success: resp.Success,
		Message: resp.Message,
	}, nil
}

func (h *AuthGRPCHandler) ResendVerificationEmail(ctx context.Context, req *pb.ResendVerificationEmailRequest) (*pb.ResendVerificationEmailResponse, error) {
	resp, err := h.authService.ResendVerificationEmail(ctx, &dto.ResendVerificationEmailRequest{
		Email: req.GetEmail(),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &pb.ResendVerificationEmailResponse{
		Success: resp.Success,
		Message: resp.Message,
	}, nil
}

func (h *AuthGRPCHandler) ForgotPassword(ctx context.Context, req *pb.ForgotPasswordRequest) (*pb.ForgotPasswordResponse, error) {
	resp, err := h.authService.ForgotPassword(ctx, &dto.ForgotPasswordRequest{
		Email:    req.GetEmail(),
		ClientIP: req.GetClientIp(),
		Role:     fromPBRole(req.GetRole()),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &pb.ForgotPasswordResponse{
		Success: resp.Success,
		Message: resp.Message,
	}, nil
}

func (h *AuthGRPCHandler) ResetPasswordWithOtp(ctx context.Context, req *pb.ResetPasswordWithOtpRequest) (*pb.ResetPasswordResponse, error) {
	resp, err := h.authService.ResetPasswordWithOtp(ctx, &dto.ResetPasswordWithOtpRequest{
		Email:       req.GetEmail(),
		OTP:         req.GetOtp(),
		NewPassword: req.GetNewPassword(),
		Role:        fromPBRole(req.GetRole()),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &pb.ResetPasswordResponse{
		Success: resp.Success,
		Message: resp.Message,
	}, nil
}

func (h *AuthGRPCHandler) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest) (*pb.ChangePasswordResponse, error) {
	userID := req.GetUserId()
	resp, err := h.authService.ChangePassword(ctx, &dto.ChangePasswordRequest{
		UserID:      userID,
		OldPassword: req.GetOldPassword(),
		NewPassword: req.GetNewPassword(),
	})
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &pb.ChangePasswordResponse{
		Success: resp.Success,
		Message: resp.Message,
	}, nil
}

func toPBRole(roleStr string) pb.Role {
	switch strings.ToUpper(strings.TrimSpace(roleStr)) {
	case "USER", "CUSTOMER":
		return pb.Role_USER
	case "MERCHANT":
		return pb.Role_MERCHANT
	case "DELIVERY_PARTNER", "DELIVERYPARTNER", "DELIVERY":
		return pb.Role_DELIVERY_PARTNER
	case "ADMIN":
		return pb.Role_ADMIN
	default:
		return pb.Role_ROLE_UNSPECIFIED
	}
}

func fromPBRole(r pb.Role) string {
	switch r {
	case pb.Role_USER:
		return "USER"
	case pb.Role_MERCHANT:
		return "MERCHANT"
	case pb.Role_DELIVERY_PARTNER:
		return "DELIVERY_PARTNER"
	case pb.Role_ADMIN:
		return "ADMIN"
	default:
		return ""
	}
}

