package middleware

import (
	"context"
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

// UnaryOwnershipInterceptor enforces that only the logged-in merchant (owner)
// or an ADMIN can get, update, and delete merchant details.
func UnaryOwnershipInterceptor(repo repository.MerchantRepository) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		userID, role := ExtractIdentity(ctx)

		switch info.FullMethod {
		case "/gocart.merchant.v1.MerchantService/GetMerchant":
			if r, ok := req.(*merchantpb.GetMerchantRequest); ok && r != nil {
				if err := verifyMerchantOwnership(ctx, repo, r.Id, userID, role); err != nil {
					return nil, err
				}
			}

		case "/gocart.merchant.v1.MerchantService/GetMerchantByUserID":
			if r, ok := req.(*merchantpb.GetMerchantByUserIDRequest); ok && r != nil {
				if role != "ADMIN" {
					if role != "MERCHANT" || userID == "" || !strings.EqualFold(r.UserId, userID) {
						return nil, status.Error(codes.PermissionDenied, "forbidden: you can only access your own merchant profile")
					}
				}
			}

		case "/gocart.merchant.v1.MerchantService/UpdateMerchant":
			if r, ok := req.(*merchantpb.UpdateMerchantRequest); ok && r != nil {
				if err := verifyMerchantOwnership(ctx, repo, r.Id, userID, role); err != nil {
					return nil, err
				}
			}

		case "/gocart.merchant.v1.MerchantService/DeleteMerchant":
			if r, ok := req.(*merchantpb.DeleteMerchantRequest); ok && r != nil {
				if err := verifyMerchantOwnership(ctx, repo, r.Id, userID, role); err != nil {
					return nil, err
				}
			}

		case "/gocart.merchant.v1.MerchantService/UpdateMerchantStatus":
			if role != "ADMIN" {
				return nil, status.Error(codes.PermissionDenied, "forbidden: only admins can update merchant status")
			}

		case "/gocart.merchant.v1.MerchantService/ListMerchants":
			if role != "ADMIN" {
				return nil, status.Error(codes.PermissionDenied, "forbidden: only admins can list all merchants")
			}

		case "/gocart.merchant.v1.MerchantService/CreateMerchant":
			if role != "" && role != "MERCHANT" && role != "ADMIN" {
				return nil, status.Error(codes.PermissionDenied, "forbidden: insufficient permissions to create merchant")
			}
		}

		return handler(ctx, req)
	}
}

func verifyMerchantOwnership(ctx context.Context, repo repository.MerchantRepository, merchantIDStr, callerUserID, callerRole string) error {
	if callerRole == "ADMIN" {
		return nil
	}
	if callerRole != "MERCHANT" || callerUserID == "" {
		return status.Error(codes.PermissionDenied, "forbidden: you can only access/modify your own merchant profile")
	}

	mID, err := uuid.Parse(merchantIDStr)
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid merchant id format")
	}

	merchant, err := repo.GetByID(ctx, mID)
	if err != nil {
		return status.Error(codes.NotFound, "merchant not found")
	}

	if !strings.EqualFold(merchant.ID.String(), callerUserID) {
		return status.Error(codes.PermissionDenied, "forbidden: you can only access/modify your own merchant profile")
	}

	return nil
}
