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
	gwGraphQL "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql"
	gwResolver "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	gwGRPC "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type mockAuthBackend struct {
	authpb.UnimplementedAuthServiceServer
}

func (m *mockAuthBackend) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.AuthResponse, error) {
	if req.IsMerchant {
		return &authpb.AuthResponse{
			AccessToken:   "jwt-test-token-12345",
			RefreshToken:  "refresh-test-token-67890",
			TokenType:     "Bearer",
			ExpiresIn:     900,
			UserId:        "01a0cdc0-6657-7667-8fc0-6f896e6f6c03",
			Role:          "MERCHANT",
			MerchantId:    "01a0cdc0-6657-7668-8387-5dbbc958aa64",
			BusinessEmail: req.Email,
			FirstName:     req.FirstName,
			LastName:      req.LastName,
		}, nil
	}
	return &authpb.AuthResponse{
		AccessToken:  "jwt-test-token-customer",
		RefreshToken: "refresh-test-token-customer",
		TokenType:    "Bearer",
		ExpiresIn:    900,
		UserId:       "01a0cdc0-customer-id",
		Role:         "CUSTOMER",
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
					firstName: "mohan"
					lastName: "kumar"
					isMerchant: true
				}
			) {
				token
				user {
					id
					email
					firstName
					lastName
					role
				}
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
				User  struct {
					ID        string `json:"id"`
					Email     string `json:"email"`
					FirstName string `json:"firstName"`
					LastName  string `json:"lastName"`
					Role      string `json:"role"`
				} `json:"user"`
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
	if reg.User.Email != "mohankumar1@gmail.com" {
		t.Errorf("expected email mohankumar1@gmail.com, got %s", reg.User.Email)
	}
	if reg.User.FirstName != "mohan" {
		t.Errorf("expected firstName mohan, got %s", reg.User.FirstName)
	}
	if reg.User.LastName != "kumar" {
		t.Errorf("expected lastName kumar, got %s", reg.User.LastName)
	}
	if reg.User.Role != "MERCHANT" {
		t.Errorf("expected role MERCHANT, got %s", reg.User.Role)
	}
}
