package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestSharedRolesAccess_TokensAndMetadata(t *testing.T) {
	secret := "shared-roles-integration-test-secret"
	ttl := 15 * time.Minute

	roles := []struct {
		name       string
		role       string
		userID     string
		expectPart bool
		expectMerc bool
	}{
		{name: "Customer", role: auth.RoleCustomer, userID: "c-user-1", expectPart: false, expectMerc: false},
		{name: "Merchant", role: auth.RoleMerchant, userID: "m-user-1", expectPart: false, expectMerc: true},
		{name: "DeliveryPartner", role: auth.RoleDeliveryPartner, userID: "d-user-1", expectPart: true, expectMerc: false},
	}

	for _, tc := range roles {
		t.Run(tc.name+" token claims contain valid customerID for shopping", func(t *testing.T) {
			uCtx := auth.UserContext{
				UserID: tc.userID,
				Role:   tc.role,
				Email:  tc.userID + "@gocart.test",
			}
			tok, err := auth.GenerateToken(uCtx, secret, ttl)
			if err != nil {
				t.Fatalf("failed to generate token: %v", err)
			}

			validated, err := auth.ValidateToken(tok, secret)
			if err != nil {
				t.Fatalf("failed to validate token: %v", err)
			}

			if validated.UserID != tc.userID {
				t.Errorf("expected user_id %s, got %s", tc.userID, validated.UserID)
			}
			if validated.CustomerID != tc.userID {
				t.Errorf("expected customer_id to be populated with user_id %s, got %s", tc.userID, validated.CustomerID)
			}
			if tc.expectMerc && validated.MerchantID != tc.userID {
				t.Errorf("expected merchant_id %s, got %s", tc.userID, validated.MerchantID)
			}
			if tc.expectPart && validated.PartnerID != tc.userID {
				t.Errorf("expected partner_id %s, got %s", tc.userID, validated.PartnerID)
			}
		})
	}
}

func TestSharedRolesAccess_InterceptorMetadataPropagation(t *testing.T) {
	interceptor := grpcclient.UnaryClientInterceptor(5 * time.Second)

	testCases := []struct {
		role   string
		userID string
		email  string
	}{
		{role: auth.RoleCustomer, userID: "cust-01", email: "cust01@example.com"},
		{role: auth.RoleMerchant, userID: "merch-01", email: "merch01@example.com"},
		{role: auth.RoleDeliveryPartner, userID: "dp-01", email: "dp01@example.com"},
	}

	for _, tc := range testCases {
		t.Run("Metadata propagated for "+tc.role, func(t *testing.T) {
			ctx := auth.WithUser(context.Background(), &auth.UserContext{
				UserID: tc.userID,
				Role:   tc.role,
				Email:  tc.email,
			})

			invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
				md, ok := metadata.FromOutgoingContext(ctx)
				if !ok {
					t.Fatalf("expected outgoing metadata in context")
				}
				if uids := md.Get(grpcclient.HeaderUserID); len(uids) == 0 || uids[0] != tc.userID {
					t.Errorf("expected header %s = %s, got %v", grpcclient.HeaderUserID, tc.userID, uids)
				}
				if roles := md.Get(grpcclient.HeaderUserRole); len(roles) == 0 || roles[0] != tc.role {
					t.Errorf("expected header %s = %s, got %v", grpcclient.HeaderUserRole, tc.role, roles)
				}
				if emails := md.Get(grpcclient.HeaderUserEmail); len(emails) == 0 || emails[0] != tc.email {
					t.Errorf("expected header %s = %s, got %v", grpcclient.HeaderUserEmail, tc.email, emails)
				}
				return nil
			}

			// Call interceptor
			_ = interceptor(ctx, "/test.Service/TestMethod", nil, nil, nil, invoker)
		})
	}
}

