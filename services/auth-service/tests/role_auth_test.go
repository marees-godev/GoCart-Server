package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	pb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
	authGRPC "github.com/marees-godev/GoCart-Server/services/auth-service/internal/handler/grpc"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRoleAuthenticationFlows(t *testing.T) {
	repo := newInMemoryAuthRepo()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-role-auth",
			ExpiryMinutes: 15,
		},
	}
	mockUser := &mockUserServiceClient{}
	mockMerchant := &mockMerchantClient{}
	authSvc := service.NewAuthService(repo, cfg, nil, mockUser, mockMerchant)
	handler := authGRPC.NewAuthGRPCHandler(authSvc, nil)

	ctx := context.Background()

	t.Run("Register and Login all three roles", func(t *testing.T) {
		roles := []struct {
			role      pb.Role
			email     string
			firstName string
			lastName  string
		}{
			{role: pb.Role_USER, email: "user@example.com", firstName: "Regular", lastName: "User"},
			{role: pb.Role_MERCHANT, email: "merchant@example.com", firstName: "Merchant", lastName: "Business"},
			{role: pb.Role_DELIVERY_PARTNER, email: "partner@example.com", firstName: "Fast", lastName: "Logistics"},
			{role: pb.Role_ADMIN, email: "admin@example.com", firstName: "System", lastName: "Admin"},
		}

		for _, tc := range roles {
			regResp, err := handler.Register(ctx, &pb.RegisterRequest{
				Email:     tc.email,
				Password:  "Password123!",
				Role:      tc.role,
				FirstName: tc.firstName,
				LastName:  tc.lastName,
			})
			if err != nil {
				t.Fatalf("failed to register role %s: %v", tc.role, err)
			}
			if regResp.AccessToken == "" {
				t.Fatalf("expected non-empty access token for role %s", tc.role)
			}
			if regResp.Role != tc.role {
				t.Fatalf("expected response role %s, got %s", tc.role, regResp.Role)
			}

			// Validate JWT Claims for registration
			claims, err := auth.ValidateToken(regResp.AccessToken, cfg.JWT.Secret)
			if err != nil {
				t.Fatalf("failed to validate token for %s: %v", tc.role, err)
			}
			if !strings.EqualFold(claims.Role, tc.role.String()) {
				t.Fatalf("expected token role claim %s, got %s", tc.role, claims.Role)
			}

			switch tc.role {
			case pb.Role_USER:
				if claims.CustomerID == "" {
					t.Errorf("expected customer_id claim in USER token")
				}
			case pb.Role_MERCHANT:
				if claims.MerchantID == "" {
					t.Errorf("expected merchant_id claim in MERCHANT token")
				}
			case pb.Role_DELIVERY_PARTNER:
				if claims.PartnerID == "" {
					t.Errorf("expected partner_id claim in DELIVERY_PARTNER token")
				}
			}

			// Mark email as verified for login simulation
			for _, cred := range repo.byEmailRole {
				if strings.EqualFold(cred.Email, tc.email) {
					cred.EmailVerified = true
				}
			}

			// Test Login with same role
			loginResp, err := handler.Login(ctx, &pb.LoginRequest{
				Email:    tc.email,
				Password: "Password123!",
				Role:     tc.role,
			})
			if err != nil {
				t.Fatalf("failed to login role %s: %v", tc.role, err)
			}
			if loginResp.AccessToken == "" {
				t.Fatalf("expected non-empty access token on login for role %s", tc.role)
			}
			if loginResp.Role != tc.role {
				t.Fatalf("expected login role %s, got %s", tc.role, loginResp.Role)
			}

			loginClaims, err := auth.ValidateToken(loginResp.AccessToken, cfg.JWT.Secret)
			if err != nil {
				t.Fatalf("failed to validate login token for %s: %v", tc.role, err)
			}
			if !strings.EqualFold(loginClaims.Role, tc.role.String()) {
				t.Fatalf("expected login token role claim %s, got %s", tc.role, loginClaims.Role)
			}

			switch tc.role {
			case pb.Role_USER:
				if loginClaims.CustomerID == "" {
					t.Errorf("expected customer_id claim in USER login token")
				}
			case pb.Role_MERCHANT:
				if loginClaims.MerchantID == "" {
					t.Errorf("expected merchant_id claim in MERCHANT login token")
				}
			case pb.Role_DELIVERY_PARTNER:
				if loginClaims.PartnerID == "" {
					t.Errorf("expected partner_id claim in DELIVERY_PARTNER login token")
				}
			}
		}
	})

	t.Run("Reject login when role mismatches", func(t *testing.T) {
		// Attempt to login as MERCHANT using USER's registered email
		_, err := handler.Login(ctx, &pb.LoginRequest{
			Email:    "user@example.com",
			Password: "Password123!",
			Role:     pb.Role_MERCHANT,
		})
		if err == nil {
			t.Fatalf("expected login rejection for mismatched role, but got success")
		}
		st, ok := status.FromError(err)
		if !ok || (st.Code() != codes.NotFound && st.Code() != codes.Unauthenticated && st.Code() != codes.InvalidArgument) {
			t.Errorf("expected not found or unauthenticated gRPC status, got %v", err)
		}

		// Attempt to login as DELIVERY_PARTNER using MERCHANT's registered email
		_, err = handler.Login(ctx, &pb.LoginRequest{
			Email:    "merchant@example.com",
			Password: "Password123!",
			Role:     pb.Role_DELIVERY_PARTNER,
		})
		if err == nil {
			t.Fatalf("expected login rejection for mismatched role, but got success")
		}
	})

	t.Run("PartnerRegistered event is emitted on partner signup", func(t *testing.T) {
		found := false
		for _, evt := range repo.outboxEvents {
			if evt.EventType == "PartnerRegistered" {
				found = true
				if evt.Topic != events.TopicPartnerRegistered && evt.Topic != "auth.partner.registered" {
					t.Errorf("expected topic %s, got %s", events.TopicPartnerRegistered, evt.Topic)
				}
				break
			}
		}
		if !found {
			t.Errorf("expected PartnerRegistered outbox event to be stored")
		}
	})

	t.Run("Register DELIVERY_PARTNER with email, password, and role alone", func(t *testing.T) {
		regResp, err := handler.Register(ctx, &pb.RegisterRequest{
			Email:    "partneralone@example.com",
			Password: "Password123!",
			Role:     pb.Role_DELIVERY_PARTNER,
		})
		if err != nil {
			t.Fatalf("failed to register delivery partner with email, password, role alone: %v", err)
		}
		if regResp.AccessToken == "" {
			t.Fatalf("expected non-empty access token")
		}
		if regResp.Role != pb.Role_DELIVERY_PARTNER {
			t.Fatalf("expected response role DELIVERY_PARTNER, got %s", regResp.Role)
		}
		if regResp.PartnerId == "" {
			t.Fatalf("expected non-empty partnerId in response")
		}

		claims, err := auth.ValidateToken(regResp.AccessToken, cfg.JWT.Secret)
		if err != nil {
			t.Fatalf("failed to validate token: %v", err)
		}
		if claims.PartnerID == "" {
			t.Errorf("expected partner_id claim in DELIVERY_PARTNER token")
		}
	})

	t.Run("ForgotPassword and ResetPasswordWithOtp with Role enum", func(t *testing.T) {
		fpResp, err := handler.ForgotPassword(ctx, &pb.ForgotPasswordRequest{
			Email: "partneralone@example.com",
			Role:  pb.Role_DELIVERY_PARTNER,
		})
		if err != nil {
			t.Fatalf("failed to request forgot password: %v", err)
		}
		if !fpResp.Success {
			t.Errorf("expected forgot password success true")
		}

		// Attempt reset password with invalid OTP to verify handler accepts Role enum
		_, err = handler.ResetPasswordWithOtp(ctx, &pb.ResetPasswordWithOtpRequest{
			Email:       "partneralone@example.com",
			Otp:         "000000",
			NewPassword: "BrandNewPass123!",
			Role:        pb.Role_DELIVERY_PARTNER,
		})
		if err == nil {
			t.Fatal("expected error for invalid otp code")
		}
	})
}

