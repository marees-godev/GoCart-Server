package resolvers

import (
	"context"

	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
)

func (r *Resolver) ensureUserProfileExists(ctx context.Context, userID string) {
	if r == nil || r.Clients == nil || r.Clients.UserClient == nil || userID == "" {
		return
	}
	if _, err := r.Clients.UserClient.GetUser(ctx, &userpb.GetUserRequest{Id: userID}); err == nil {
		return
	}

	userCtx, _ := auth.UserFromContext(ctx)
	email := ""
	firstName := "User"
	lastName := "Account"
	if userCtx != nil && userCtx.Email != "" {
		email = userCtx.Email
	}

	if r.Clients.MerchantClient != nil {
		mResp, mErr := r.Clients.MerchantClient.GetMerchant(ctx, &merchantpb.GetMerchantRequest{Id: userID})
		if mErr == nil && mResp != nil && mResp.Merchant != nil {
			if mResp.Merchant.BusinessEmail != "" {
				email = mResp.Merchant.BusinessEmail
			}
			if mResp.Merchant.FirstName != "" {
				firstName = mResp.Merchant.FirstName
			}
			if mResp.Merchant.LastName != "" {
				lastName = mResp.Merchant.LastName
			}
		}
	}

	if (email == "" || firstName == "User") && r.Clients.DeliveryClient != nil {
		dResp, dErr := r.Clients.DeliveryClient.GetPartnerProfile(ctx, &deliverypb.GetPartnerProfileRequest{PartnerId: userID})
		if dErr == nil && dResp != nil && dResp.Profile != nil {
			if dResp.Profile.Email != "" {
				email = dResp.Profile.Email
			}
			if dResp.Profile.CompanyName != "" {
				firstName = dResp.Profile.CompanyName
				lastName = "Partner"
			}
		}
	}

	if email == "" {
		email = userID + "@gocart.internal"
	}

	_, _ = r.Clients.UserClient.CreateUser(ctx, &userpb.CreateUserRequest{
		Id:        userID,
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
	})
}
