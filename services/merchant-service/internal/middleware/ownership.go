package middleware

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/repository"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func ExtractIdentity(ctx context.Context) (userID, role string) {
	if u, ok := auth.FromContext(ctx); ok && u != nil {
		userID = strings.TrimSpace(u.UserID)
		role = strings.ToUpper(strings.TrimSpace(u.Role))
	}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if userID == "" {
			for _, key := range []string{"x-user-id", "user-id", "user_id"} {
				if vals := md.Get(key); len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
					userID = strings.TrimSpace(vals[0])
					break
				}
			}
		}
		if role == "" {
			for _, key := range []string{"x-user-role", "user-role", "user_role", "role"} {
				if vals := md.Get(key); len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
					role = strings.ToUpper(strings.TrimSpace(vals[0]))
					break
				}
			}
		}
	}
	return userID, role
}

// UnaryOwnershipInterceptor enforces that only the logged-in merchant (owner),
// internal service consumer, or an ADMIN can get, update, and delete merchant details.
func UnaryOwnershipInterceptor(repo repository.MerchantRepository, log ...*slog.Logger) grpc.UnaryServerInterceptor {
	var l *slog.Logger
	if len(log) > 0 && log[0] != nil {
		l = log[0]
	} else {
		l = slog.Default()
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		userID, role := ExtractIdentity(ctx)

		switch {
		case strings.HasSuffix(info.FullMethod, "/GetMerchant"):
			if r, ok := req.(*merchantpb.GetMerchantRequest); ok && r != nil {
				if err := verifyMerchantOwnership(ctx, repo, r.Id, userID, role, l); err != nil {
					return nil, err
				}
			}

		case strings.HasSuffix(info.FullMethod, "/UpdateMerchant"):
			if r, ok := req.(*merchantpb.UpdateMerchantRequest); ok && r != nil {
				if err := verifyMerchantOwnership(ctx, repo, r.Id, userID, role, l); err != nil {
					return nil, err
				}
			}

		case strings.HasSuffix(info.FullMethod, "/DeleteMerchant"):
			if r, ok := req.(*merchantpb.DeleteMerchantRequest); ok && r != nil {
				if err := verifyDeleteMerchantOwnership(ctx, repo, r.Id, userID, role, l); err != nil {
					return nil, err
				}
			}

		case strings.HasSuffix(info.FullMethod, "/UpdateMerchantStatus"):
			if role != "ADMIN" {
				l.Warn("Ownership: non-admin attempted to update merchant status", slog.String("role", role), slog.String("user_id", userID))
				return nil, status.Error(codes.PermissionDenied, "forbidden: only admins can update merchant status")
			}

		case strings.HasSuffix(info.FullMethod, "/ListMerchants"):
			if role != "ADMIN" {
				l.Warn("Ownership: non-admin attempted to list merchants", slog.String("role", role), slog.String("user_id", userID))
				return nil, status.Error(codes.PermissionDenied, "forbidden: only admins can list all merchants")
			}

		case strings.HasSuffix(info.FullMethod, "/CreateMerchant"):
			if role != "" && role != "MERCHANT" && role != "ADMIN" {
				l.Warn("Ownership: invalid caller role to create merchant", slog.String("role", role), slog.String("user_id", userID))
				return nil, status.Error(codes.PermissionDenied, "forbidden: insufficient permissions to create merchant")
			}
		}

		return handler(ctx, req)
	}
}

func verifyMerchantOwnership(ctx context.Context, repo repository.MerchantRepository, merchantIDStr, callerUserID, callerRole string, l *slog.Logger) error {
	if callerRole == "ADMIN" || (callerRole == "" && callerUserID == "") {
		return nil
	}
	if callerRole != "MERCHANT" || callerUserID == "" {
		l.Warn("Ownership check failed: not a merchant or missing caller ID", slog.String("caller_role", callerRole), slog.String("caller_user_id", callerUserID))
		return status.Error(codes.PermissionDenied, "forbidden: you can only access/modify your own merchant profile")
	}

	mID, err := uuid.Parse(merchantIDStr)
	if err != nil {
		l.Warn("Ownership check failed: invalid merchant ID format", slog.String("merchant_id", merchantIDStr))
		return status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	merchant, err := repo.GetByID(ctx, mID)
	if err != nil {
		l.Warn("Ownership check failed: merchant not found", slog.String("merchant_id", merchantIDStr))
		return status.Error(codes.NotFound, "merchant not found")
	}

	if !strings.EqualFold(merchant.ID.String(), callerUserID) {
		l.Warn("Ownership check failed: caller ID does not match merchant ID", slog.String("merchant_id", merchant.ID.String()), slog.String("caller_user_id", callerUserID))
		return status.Error(codes.PermissionDenied, "forbidden: you can only access/modify your own merchant profile")
	}

	return nil
}

func verifyDeleteMerchantOwnership(ctx context.Context, repo repository.MerchantRepository, merchantIDStr, callerUserID, callerRole string, l *slog.Logger) error {
	if callerRole == "ADMIN" {
		l.Warn("Ownership: admin attempted to delete merchant account", slog.String("caller_user_id", callerUserID))
		return status.Error(codes.PermissionDenied, "forbidden: admins cannot delete merchant accounts, admins can only suspend merchants")
	}
	if callerRole != "MERCHANT" || callerUserID == "" {
		l.Warn("Ownership check failed: not a merchant or missing caller ID for deletion", slog.String("caller_role", callerRole), slog.String("caller_user_id", callerUserID))
		return status.Error(codes.PermissionDenied, "forbidden: only the merchant who created the account can delete it")
	}

	mID, err := uuid.Parse(merchantIDStr)
	if err != nil {
		l.Warn("Ownership check failed: invalid merchant ID format", slog.String("merchant_id", merchantIDStr))
		return status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	merchant, err := repo.GetByID(ctx, mID)
	if err != nil {
		l.Warn("Ownership check failed: merchant not found", slog.String("merchant_id", merchantIDStr))
		return status.Error(codes.NotFound, "merchant not found")
	}

	if !strings.EqualFold(merchant.ID.String(), callerUserID) {
		l.Warn("Ownership check failed: caller ID does not match merchant ID for deletion", slog.String("merchant_id", merchant.ID.String()), slog.String("caller_user_id", callerUserID))
		return status.Error(codes.PermissionDenied, "forbidden: only the merchant who created the account can delete it")
	}

	return nil
}

