package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/client"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	gwGraphQL "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql"
	gwResolver "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type mockCartBackend struct {
	cartpb.UnimplementedCartServiceServer
	carts map[string]*cartpb.Cart
}

func newMockCartBackend() *mockCartBackend {
	return &mockCartBackend{
		carts: make(map[string]*cartpb.Cart),
	}
}

func (m *mockCartBackend) GetCart(ctx context.Context, req *cartpb.GetCartRequest) (*cartpb.GetCartResponse, error) {
	if req.UserId == "err-not-found" {
		return nil, status.Error(codes.NotFound, "cart record not found")
	}
	if req.UserId == "err-invalid" {
		return nil, status.Error(codes.InvalidArgument, "invalid user id format")
	}
	if req.UserId == "err-unavailable" {
		return nil, status.Error(codes.Unavailable, "cart database connection failed")
	}

	cart, exists := m.carts[req.UserId]
	if !exists {
		cart = &cartpb.Cart{
			Id:          "cart-auto-" + req.UserId,
			UserId:      req.UserId,
			TotalAmount: 0,
			Items:       []*cartpb.CartItem{},
		}
	}
	return &cartpb.GetCartResponse{Cart: cart}, nil
}

func (m *mockCartBackend) AddToCart(ctx context.Context, req *cartpb.AddToCartRequest) (*cartpb.AddToCartResponse, error) {
	if req.Quantity <= 0 {
		return nil, status.Error(codes.InvalidArgument, "quantity must be greater than 0")
	}

	cart, exists := m.carts[req.UserId]
	if !exists {
		cart = &cartpb.Cart{
			Id:          "cart-" + req.UserId,
			UserId:      req.UserId,
			TotalAmount: 0,
			Items:       []*cartpb.CartItem{},
		}
		m.carts[req.UserId] = cart
	}

	newItem := &cartpb.CartItem{
		Id:        fmt.Sprintf("item-%d", len(cart.Items)+1),
		ProductId: req.ProductId,
		Quantity:  req.Quantity,
		UnitPrice: req.UnitPrice,
	}
	cart.Items = append(cart.Items, newItem)
	cart.TotalAmount += float64(req.Quantity) * req.UnitPrice

	return &cartpb.AddToCartResponse{Cart: cart}, nil
}

func setupIntegrationApp(t *testing.T, backend *mockCartBackend) (app *fiber.App,_ func()) {
	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	cartpb.RegisterCartServiceServer(grpcServer, backend)

	go func() {
		_ = grpcServer.Serve(lis)
	}()

	cfg := &config.Config{
		App: config.AppConfig{
			Name:        "api-gateway",
			Environment: "test",
			Version:     "1.0.0",
		},
		GRPC: config.GRPCConfig{
			CartServiceAddr: "passthrough://bufnet",
			DefaultTimeout:  2 * time.Second,
		},
	}

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	clientMgr, err := client.NewClientManager(cfg, grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("failed to create client manager: %v", err)
	}

	gqlResolver := gwResolver.NewResolver(nil, "1.0.0")
	gqlServer := gwGraphQL.NewServer(gqlResolver)
	app.All("/graphql", adaptor.HTTPHandler(gqlServer))

	cleanup := func() {
		clientMgr.Close()
		grpcServer.Stop()
		_ = lis.Close()
	}

	return app, cleanup
}

func TestE2E_ResolverClientService_AddToCartAndGetCart(t *testing.T) {
	backend := newMockCartBackend()
	app, cleanup := setupIntegrationApp(t, backend)
	defer cleanup()

	// 1. Mutation: AddToCart
	mutationQuery := `
		mutation {
			addToCart(input: {
				userId: "user-42"
				productId: "prod-99"
				quantity: 2
				unitPrice: 15.5
			}) {
				id
				userId
				totalAmount
				items {
					productId
					quantity
					unitPrice
				}
			}
		}
	`

	reqBody, _ := json.Marshal(map[string]string{"query": mutationQuery})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to execute AddToCart mutation: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	var mutationResult struct {
		Data struct {
			AddToCart struct {
				ID          string  `json:"id"`
				UserID      string  `json:"userId"`
				TotalAmount float64 `json:"totalAmount"`
				Items       []struct {
					ProductID string  `json:"productId"`
					Quantity  int     `json:"quantity"`
					UnitPrice float64 `json:"unitPrice"`
				} `json:"items"`
			} `json:"addToCart"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	if err := json.Unmarshal(bodyBytes, &mutationResult); err != nil {
		t.Fatalf("failed to decode response: %v, body: %s", err, string(bodyBytes))
	}
	if len(mutationResult.Errors) > 0 {
		t.Fatalf("unexpected GraphQL errors: %v", mutationResult.Errors)
	}

	if mutationResult.Data.AddToCart.UserID != "user-42" {
		t.Errorf("expected userId user-42, got %s", mutationResult.Data.AddToCart.UserID)
	}
	if mutationResult.Data.AddToCart.TotalAmount != 31.0 {
		t.Errorf("expected totalAmount 31.0, got %f", mutationResult.Data.AddToCart.TotalAmount)
	}
	if len(mutationResult.Data.AddToCart.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(mutationResult.Data.AddToCart.Items))
	}

	// 2. Query: Cart
	queryCart := `
		query {
			cart(userId: "user-42") {
				id
				userId
				totalAmount
				items {
					productId
					quantity
				}
			}
		}
	`
	reqBody, _ = json.Marshal(map[string]string{"query": queryCart})
	req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to execute cart query: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)

	var queryResult struct {
		Data struct {
			Cart struct {
				ID          string  `json:"id"`
				UserID      string  `json:"userId"`
				TotalAmount float64 `json:"totalAmount"`
			} `json:"cart"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	_ = json.Unmarshal(bodyBytes, &queryResult)
	if len(queryResult.Errors) > 0 {
		t.Fatalf("unexpected GraphQL errors in cart query: %v", queryResult.Errors)
	}
	if queryResult.Data.Cart.UserID != "user-42" {
		t.Errorf("expected cart userId user-42, got %s", queryResult.Data.Cart.UserID)
	}
	if queryResult.Data.Cart.TotalAmount != 31.0 {
		t.Errorf("expected cart totalAmount 31.0, got %f", queryResult.Data.Cart.TotalAmount)
	}
}

func TestE2E_DownstreamFailureHandlingAndErrorTranslation(t *testing.T) {
	backend := newMockCartBackend()
	app, cleanup := setupIntegrationApp(t, backend)
	defer cleanup()

	tests := []struct {
		name         string
		targetUser   string
		expectedCode string
	}{
		{
			name:         "Translates NotFound gRPC error",
			targetUser:   "err-not-found",
			expectedCode: appErrors.CodeNotFound,
		},
		{
			name:         "Translates InvalidArgument gRPC error",
			targetUser:   "err-invalid",
			expectedCode: appErrors.CodeBadRequest,
		},
		{
			name:         "Translates Unavailable gRPC error gracefully without crashing",
			targetUser:   "err-unavailable",
			expectedCode: appErrors.CodeServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := fmt.Sprintf(`query { cart(userId: "%s") { id } }`, tt.targetUser)
			reqBody, _ := json.Marshal(map[string]string{"query": query})
			req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("gateway request failed: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected status 200 with GraphQL errors, got %d", resp.StatusCode)
			}

			bodyBytes, _ := io.ReadAll(resp.Body)
			var gqlResp struct {
				Data   any `json:"data"`
				Errors []struct {
					Message    string         `json:"message"`
					Extensions map[string]any `json:"extensions"`
				} `json:"errors"`
			}
			if err := json.Unmarshal(bodyBytes, &gqlResp); err != nil {
				t.Fatalf("failed to decode response: %v, body: %s", err, string(bodyBytes))
			}

			if len(gqlResp.Errors) == 0 {
				t.Fatalf("expected GraphQL errors, got none. body: %s", string(bodyBytes))
			}

			code, _ := gqlResp.Errors[0].Extensions["code"].(string)
			if code != tt.expectedCode {
				t.Errorf("expected error extension code %q, got %q (message: %s)",
					tt.expectedCode, code, gqlResp.Errors[0].Message)
			}
		})
	}
}
