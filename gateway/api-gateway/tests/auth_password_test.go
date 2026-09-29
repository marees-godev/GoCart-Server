package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	gwConfig "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	gwGraphQL "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/generated"
	gwResolver "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	gwGRPC "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type mockPasswordAuthBackend struct {
	authpb.UnimplementedAuthServiceServer
}

func (m *mockPasswordAuthBackend) ForgotPassword(ctx context.Context, req *authpb.ForgotPasswordRequest) (*authpb.ForgotPasswordResponse, error) {
	if req.Email == "" {
		return nil, appErrors.MapAppErrorToGRPC(appErrors.BadRequest("email is required"))
	}
	return &authpb.ForgotPasswordResponse{
		Success: true,
		Message: "If an account exists with this email address, a password reset code has been sent.",
	}, nil
}

func (m *mockPasswordAuthBackend) ResetPasswordWithOtp(ctx context.Context, req *authpb.ResetPasswordWithOtpRequest) (*authpb.ResetPasswordResponse, error) {
	if req.Otp != "123456" {
		return nil, appErrors.MapAppErrorToGRPC(appErrors.BadRequest("invalid password reset code"))
	}
	return &authpb.ResetPasswordResponse{
		Success: true,
		Message: "Password has been reset successfully",
	}, nil
}

func (m *mockPasswordAuthBackend) ChangePassword(ctx context.Context, req *authpb.ChangePasswordRequest) (*authpb.ChangePasswordResponse, error) {
	if req.OldPassword != "CurrentPass123!" {
		return nil, appErrors.MapAppErrorToGRPC(appErrors.InvalidCredentials("invalid current password"))
	}
	return &authpb.ChangePasswordResponse{
		Success: true,
		Message: "Password changed successfully",
	}, nil
}

func setupPasswordTestServer(t *testing.T) (*fiber.App, string) {
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	authpb.RegisterAuthServiceServer(srv, &mockPasswordAuthBackend{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() { srv.Stop() })

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial bufnet: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	authClient := authpb.NewAuthServiceClient(conn)
	clients := &gwGRPC.Clients{
		AuthClient: authClient,
	}

	cfg := &gwConfig.Config{
		JWT: gwConfig.JWTConfig{
			Secret: "test-jwt-secret-key-1234567890123456",
		},
	}

	resolver := gwResolver.NewResolver(clients, "1.0.0")
	schema := generated.NewExecutableSchema(generated.Config{Resolvers: resolver})
	handler := gwGraphQL.NewHandler(schema, cfg)

	app := fiber.New()
	app.All("/graphql", adaptor.HTTPHandler(handler))

	return app, cfg.JWT.Secret
}

func TestGraphQL_ForgotPassword(t *testing.T) {
	app, _ := setupPasswordTestServer(t)

	mutation := `
		mutation ForgotPassword {
			forgotPassword(input: { email: "user@example.com", isMerchant: false }) {
				success
				message
			}
		}
	`

	reqBody, _ := json.Marshal(map[string]string{"query": mutation})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	var result struct {
		Data struct {
			ForgotPassword struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			} `json:"forgotPassword"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		t.Fatalf("failed to parse json response: %v, raw: %s", err, string(bodyBytes))
	}

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected GraphQL errors: %v", result.Errors)
	}

	if !result.Data.ForgotPassword.Success {
		t.Errorf("expected success true, got false")
	}
	if !strings.Contains(result.Data.ForgotPassword.Message, "password reset code has been sent") {
		t.Errorf("expected enumeration defense message, got: %s", result.Data.ForgotPassword.Message)
	}
}

func TestGraphQL_ResetPasswordWithOtp_Success(t *testing.T) {
	app, _ := setupPasswordTestServer(t)

	mutation := `
		mutation ResetPassword {
			resetPasswordWithOtp(input: {
				email: "user@example.com"
				otp: "123456"
				newPassword: "NewSecurePassword123!"
				isMerchant: false
			}) {
				success
				message
			}
		}
	`

	reqBody, _ := json.Marshal(map[string]string{"query": mutation})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	var result struct {
		Data struct {
			ResetPasswordWithOtp struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			} `json:"resetPasswordWithOtp"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		t.Fatalf("failed to parse json response: %v, raw: %s", err, string(bodyBytes))
	}

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected GraphQL errors: %v", result.Errors)
	}

	if !result.Data.ResetPasswordWithOtp.Success {
		t.Errorf("expected success true, got false")
	}
}

func TestGraphQL_ResetPasswordWithOtp_InvalidOtp(t *testing.T) {
	app, _ := setupPasswordTestServer(t)

	mutation := `
		mutation ResetPassword {
			resetPasswordWithOtp(input: {
				email: "user@example.com"
				otp: "999999"
				newPassword: "NewSecurePassword123!"
				isMerchant: false
			}) {
				success
				message
			}
		}
	`

	reqBody, _ := json.Marshal(map[string]string{"query": mutation})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	var result struct {
		Data   any `json:"data"`
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		t.Fatalf("failed to parse json response: %v, raw: %s", err, string(bodyBytes))
	}

	if len(result.Errors) == 0 {
		t.Fatal("expected GraphQL errors for invalid OTP, got none")
	}
	if !strings.Contains(result.Errors[0].Message, "invalid password reset code") {
		t.Errorf("expected invalid password reset code error, got: %s", result.Errors[0].Message)
	}
}

func TestGraphQL_ChangePassword_Unauthenticated(t *testing.T) {
	app, _ := setupPasswordTestServer(t)

	mutation := `
		mutation ChangePassword {
			changePassword(input: {
				oldPassword: "CurrentPass123!"
				newPassword: "NewSecurePassword123!"
			}) {
				success
				message
			}
		}
	`

	reqBody, _ := json.Marshal(map[string]string{"query": mutation})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	var result struct {
		Data   any `json:"data"`
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		t.Fatalf("failed to parse json response: %v, raw: %s", err, string(bodyBytes))
	}

	if len(result.Errors) == 0 {
		t.Fatal("expected unauthenticated error, got none")
	}
	if result.Errors[0].Extensions.Code != "UNAUTHORIZED" {
		t.Errorf("expected error code UNAUTHORIZED, got: %s", result.Errors[0].Extensions.Code)
	}
}

func TestGraphQL_ChangePassword_WrongOldPassword(t *testing.T) {
	app, secret := setupPasswordTestServer(t)

	token, err := auth.GenerateToken(auth.UserContext{
		UserID: "01a0cdc0-customer-id",
		Email:  "user@example.com",
		Role:   "CUSTOMER",
	}, secret, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	mutation := `
		mutation ChangePassword {
			changePassword(input: {
				oldPassword: "WrongOldPassword123!"
				newPassword: "NewSecurePassword123!"
			}) {
				success
				message
			}
		}
	`

	reqBody, _ := json.Marshal(map[string]string{"query": mutation})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	var result struct {
		Data   any `json:"data"`
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		t.Fatalf("failed to parse json response: %v, raw: %s", err, string(bodyBytes))
	}

	if len(result.Errors) == 0 {
		t.Fatal("expected error for wrong old password, got none")
	}
	if result.Errors[0].Extensions.Code != "INVALID_CREDENTIALS" {
		t.Errorf("expected error code INVALID_CREDENTIALS, got: %s", result.Errors[0].Extensions.Code)
	}
}

func TestGraphQL_ChangePassword_Success(t *testing.T) {
	app, secret := setupPasswordTestServer(t)

	token, err := auth.GenerateToken(auth.UserContext{
		UserID: "01a0cdc0-customer-id",
		Email:  "user@example.com",
		Role:   "CUSTOMER",
	}, secret, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	mutation := `
		mutation ChangePassword {
			changePassword(input: {
				oldPassword: "CurrentPass123!"
				newPassword: "NewSecurePassword123!"
			}) {
				success
				message
			}
		}
	`

	reqBody, _ := json.Marshal(map[string]string{"query": mutation})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	var result struct {
		Data struct {
			ChangePassword struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			} `json:"changePassword"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		t.Fatalf("failed to parse json response: %v, raw: %s", err, string(bodyBytes))
	}

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected GraphQL errors: %v", result.Errors)
	}

	if !result.Data.ChangePassword.Success {
		t.Errorf("expected success true, got false")
	}
}
