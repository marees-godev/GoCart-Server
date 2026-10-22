package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	gwGraphQL "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql"
	gwResolver "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	gwGRPC "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type mockDeliveryBackend struct {
	deliverypb.UnimplementedDeliveryServiceServer
}

func (m *mockDeliveryBackend) CreatePartnerProfile(ctx context.Context, req *deliverypb.CreatePartnerProfileRequest) (*deliverypb.PartnerProfileResponse, error) {
	return &deliverypb.PartnerProfileResponse{
		Profile: &deliverypb.DeliveryPartnerProfile{
			Id:          "profile-uuid-1",
			PartnerId:   req.PartnerId,
			CompanyName: req.CompanyName,
			Email:       req.Email,
			IsActive:    true,
		},
	}, nil
}

type mockAuthBackend struct {
	authpb.UnimplementedAuthServiceServer
}

func (m *mockAuthBackend) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.AuthResponse, error) {
	if req.Role == authpb.Role_DELIVERY_PARTNER {
		return &authpb.AuthResponse{
			AccessToken:  "jwt-test-token-partner",
			RefreshToken: "refresh-test-token-partner",
			TokenType:    "Bearer",
			ExpiresIn:    900,
			UserId:       "01a0cdc0-partner-id",
			Role:         authpb.Role_DELIVERY_PARTNER,
			PartnerId:    "01a0cdc0-partner-id",
			FirstName:    req.FirstName,
			LastName:     req.LastName,
		}, nil
	}
	if req.Role == authpb.Role_MERCHANT {
		return &authpb.AuthResponse{
			AccessToken:   "jwt-test-token-12345",
			RefreshToken:  "refresh-test-token-67890",
			TokenType:     "Bearer",
			ExpiresIn:     900,
			UserId:        "01a0cdc0-6657-7667-8fc0-6f896e6f6c03",
			Role:          authpb.Role_MERCHANT,
			MerchantId:    "01a0cdc0-6657-7668-8387-5dbbc958aa64",
			BusinessEmail: req.Email,
			FirstName:     "mohan",
			LastName:      "kumar",
		}, nil
	}
	return &authpb.AuthResponse{
		AccessToken:  "jwt-test-token-customer",
		RefreshToken: "refresh-test-token-customer",
		TokenType:    "Bearer",
		ExpiresIn:    900,
		UserId:       "01a0cdc0-customer-id",
		Role:         authpb.Role_USER,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
	}, nil
}

func TestMerchantRegistration_ReturnsAllFields(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	authpb.RegisterAuthServiceServer(srv, &mockAuthBackend{})
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial bufnet: %v", err)
	}
	defer conn.Close()

	authClient := authpb.NewAuthServiceClient(conn)
	clients := &gwGRPC.Clients{
		AuthClient: authClient,
	}

	resolver := gwResolver.NewResolver(clients, "1.0.0")
	gqlServer := gwGraphQL.NewServer(resolver)

	app := fiber.New()
	app.All("/graphql", adaptor.HTTPHandler(gqlServer))

	mutation := `
		mutation Register2 {
			register(
				input: {
					email: "mohankumar1@gmail.com"
					password: "mojo1234"
					role: MERCHANT
					firstName: "mohan"
					lastName: "kumar"
				}
			) {
				token
				role
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
			Register struct {
				Token string `json:"token"`
				Role  string `json:"role"`
			} `json:"register"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		t.Fatalf("failed to parse json response: %v, raw: %s", err, string(bodyBytes))
	}

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected GraphQL errors: %v", result.Errors)
	}

	reg := result.Data.Register
	if reg.Token != "jwt-test-token-12345" {
		t.Errorf("expected token jwt-test-token-12345, got %s", reg.Token)
	}
	if reg.Role != "MERCHANT" {
		t.Errorf("expected role MERCHANT, got %s", reg.Role)
	}
}

func TestDeliveryPartnerRegistration_EmailPasswordRoleAlone(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	authpb.RegisterAuthServiceServer(srv, &mockAuthBackend{})
	deliverypb.RegisterDeliveryServiceServer(srv, &mockDeliveryBackend{})
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial bufnet: %v", err)
	}
	defer conn.Close()

	authClient := authpb.NewAuthServiceClient(conn)
	clients := &gwGRPC.Clients{
		AuthClient:     authClient,
		DeliveryClient: deliverypb.NewDeliveryServiceClient(conn),
	}

	resolver := gwResolver.NewResolver(clients, "1.0.0")
	gqlServer := gwGraphQL.NewServer(resolver)

	app := fiber.New()
	app.All("/graphql", adaptor.HTTPHandler(gqlServer))

	mutation := `
		mutation RegisterDeliveryPartnerAlone {
			register(
				input: {
					email: "partneralone@example.com"
					password: "PartnerPassword123!"
					role: DELIVERY_PARTNER
				}
			) {
				token
				role
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
			Register struct {
				Token string `json:"token"`
				Role  string `json:"role"`
			} `json:"register"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		t.Fatalf("failed to parse json response: %v, raw: %s", err, string(bodyBytes))
	}

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected GraphQL errors: %v", result.Errors)
	}

	reg := result.Data.Register
	if reg.Token != "jwt-test-token-partner" {
		t.Errorf("expected token jwt-test-token-partner, got %s", reg.Token)
	}
	if reg.Role != "DELIVERY_PARTNER" {
		t.Errorf("expected role DELIVERY_PARTNER, got %s", reg.Role)
	}
}
