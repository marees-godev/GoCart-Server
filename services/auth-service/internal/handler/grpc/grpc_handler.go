package grpc

import (
	"context"
	"fmt"
	"log/slog"

	pb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func mapAppErrorToGRPC(err error) error {
	if err == nil {
		return nil
	}
	appErr := appErrors.AsAppError(err)
	switch appErr.Code {
	case appErrors.CodeNotFound:
		return status.Error(codes.NotFound, appErr.Message)
	case appErrors.CodeUnauthorized:
		return status.Error(codes.Unauthenticated, appErr.Message)
	case appErrors.CodeForbidden:
		return status.Error(codes.PermissionDenied, appErr.Message)
	case appErrors.CodeBadRequest:
		return status.Error(codes.InvalidArgument, appErr.Message)
	case appErrors.CodeConflict:
		return status.Error(codes.AlreadyExists, appErr.Message)
	case appErrors.CodeServiceUnavailable:
		return status.Error(codes.Unavailable, appErr.Message)
	default:
		return status.Error(codes.Internal, appErr.Message)
	}
}

func (h *AuthGRPCHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.AuthResponse, error) {
	resp, err := h.authService.Login(ctx, &dto.LoginRequest{
		Email:    req.GetEmail(),
		Password: req.GetPassword(),
	})
	if err != nil {
		h.logger.Error("Failed to bind request", slog.Any("error", err))
		return nil, mapAppErrorToGRPC(err)
	}

	return &pb.AuthResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		TokenType:    resp.TokenType,
		ExpiresIn:    int64(resp.ExpiresIn),
		UserId:       resp.UserID,
	}, nil
}

func (h *AuthGRPCHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.AuthResponse, error) {
	fmt.Println("comess::")
	resp, err := h.authService.Register(ctx, &dto.RegisterRequest{
		Email:      req.GetEmail(),
		Password:   req.GetPassword(),
		FirstName:  req.GetFirstName(),
		LastName:   req.GetLastName(),
		IsMerchant: req.GetIsMerchant(),
	})
	if err != nil {
		h.logger.Error("Failed to bind request", slog.Any("error", err))
		return nil, mapAppErrorToGRPC(err)
	}

	return &pb.AuthResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		TokenType:    resp.TokenType,
		ExpiresIn:    int64(resp.ExpiresIn),
		UserId:       resp.UserID,
	}, nil
}

func (h *AuthGRPCHandler) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	resp, err := h.authService.ValidateToken(ctx, &dto.ValidateTokenRequest{
		Token: req.GetToken(),
	})
	if err != nil {
		h.logger.Error("Failed to bind request", slog.Any("error", err))
		return nil, mapAppErrorToGRPC(err)
	}

	return &pb.ValidateTokenResponse{
		Valid:  resp.Valid,
		UserId: resp.UserID,
		Email:  resp.Email,
		Role:   resp.Role,
	}, nil
}

func (h *AuthGRPCHandler) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.AuthResponse, error) {
	resp, err := h.authService.RefreshToken(ctx, &dto.RefreshTokenRequest{
		RefreshToken: req.GetRefreshToken(),
	})
	if err != nil {
		h.logger.Error("Failed to bind request", slog.Any("error", err))
		return nil, mapAppErrorToGRPC(err)
	}

	return &pb.AuthResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		TokenType:    resp.TokenType,
		ExpiresIn:    int64(resp.ExpiresIn),
		UserId:       resp.UserID,
	}, nil
}
