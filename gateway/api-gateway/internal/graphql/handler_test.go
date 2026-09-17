package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	gatewayGRPC "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/cartpb"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/orderpb"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/productpb"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"
	"google.golang.org/grpc"
)

// ----------------------------------------------------------------------------
// Mock gRPC Clients
// ----------------------------------------------------------------------------

type mockUserClient struct{}

func (m *mockUserClient) GetUser(ctx context.Context, in *userpb.GetUserRequest, opts ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	return &userpb.GetUserResponse{
		User: &userpb.User{
			Id:        in.Id,
			Email:     "user@example.com",
			FirstName: "Jane",
			LastName:  "Doe",
			Role:      "customer",
			CreatedAt: "2026-01-01T00:00:00Z",
		},
	}, nil
}

func (m *mockUserClient) Login(ctx context.Context, in *userpb.LoginRequest, opts ...grpc.CallOption) (*userpb.AuthResponse, error) {
	return &userpb.AuthResponse{
		Token: "jwt-mock-token",
		User: &userpb.User{
			Id:    "u-100",
			Email: in.Email,
			Role:  "customer",
		},
	}, nil
}

func (m *mockUserClient) Register(ctx context.Context, in *userpb.RegisterRequest, opts ...grpc.CallOption) (*userpb.AuthResponse, error) {
	return &userpb.AuthResponse{
		Token: "jwt-mock-register-token",
		User: &userpb.User{
			Id:        "u-101",
			Email:     in.Email,
			FirstName: in.FirstName,
			LastName:  in.LastName,
			Role:      "customer",
		},
	}, nil
}

type mockProductClient struct{}

func (m *mockProductClient) GetProduct(ctx context.Context, in *productpb.GetProductRequest, opts ...grpc.CallOption) (*productpb.GetProductResponse, error) {
	return &productpb.GetProductResponse{
		Product: &productpb.Product{
			Id:            in.Id,
			Name:          "Mock Laptop",
			Description:   "High performance laptop",
			Price:         1299.99,
			CategoryId:    "cat-tech",
			StockQuantity: 15,
			CreatedAt:     "2026-01-01T00:00:00Z",
		},
	}, nil
}

func (m *mockProductClient) ListProducts(ctx context.Context, in *productpb.ListProductsRequest, opts ...grpc.CallOption) (*productpb.ListProductsResponse, error) {
	return &productpb.ListProductsResponse{
		Products: []*productpb.Product{
			{
				Id:            "p-1",
				Name:          "Product 1",
				Price:         19.99,
				StockQuantity: 50,
			},
			{
				Id:            "p-2",
				Name:          "Product 2",
				Price:         29.99,
				StockQuantity: 30,
			},
		},
		Total: 2,
	}, nil
}

func (m *mockProductClient) CreateProduct(ctx context.Context, in *productpb.CreateProductRequest, opts ...grpc.CallOption) (*productpb.CreateProductResponse, error) {
	return &productpb.CreateProductResponse{
		Product: &productpb.Product{
			Id:            "p-new",
			Name:          in.Name,
			Description:   in.Description,
			Price:         in.Price,
			CategoryId:    in.CategoryId,
			StockQuantity: in.StockQuantity,
			CreatedAt:     "2026-09-17T00:00:00Z",
		},
	}, nil
}

type mockCartClient struct{}

func (m *mockCartClient) GetCart(ctx context.Context, in *cartpb.GetCartRequest, opts ...grpc.CallOption) (*cartpb.GetCartResponse, error) {
	return &cartpb.GetCartResponse{
		Cart: &cartpb.Cart{
			Id:     "c-1",
			UserId: in.UserId,
			Items: []*cartpb.CartItem{
				{
					Id:        "ci-1",
					ProductId: "p-1",
					Quantity:  2,
					UnitPrice: 19.99,
				},
			},
			TotalAmount: 39.98,
		},
	}, nil
}

func (m *mockCartClient) AddToCart(ctx context.Context, in *cartpb.AddToCartRequest, opts ...grpc.CallOption) (*cartpb.AddToCartResponse, error) {
	return &cartpb.AddToCartResponse{
		Cart: &cartpb.Cart{
			Id:     "c-1",
			UserId: in.UserId,
			Items: []*cartpb.CartItem{
				{
					Id:        "ci-2",
					ProductId: in.ProductId,
					Quantity:  in.Quantity,
					UnitPrice: 25.00,
				},
			},
			TotalAmount: float64(in.Quantity) * 25.00,
		},
	}, nil
}

type mockOrderClient struct{}

func (m *mockOrderClient) GetOrder(ctx context.Context, in *orderpb.GetOrderRequest, opts ...grpc.CallOption) (*orderpb.GetOrderResponse, error) {
	return &orderpb.GetOrderResponse{
		Order: &orderpb.Order{
			Id:     in.Id,
			UserId: "u-1",
			Status: "PENDING",
			Items: []*orderpb.OrderItem{
				{
					Id:        "oi-1",
					ProductId: "p-1",
					Quantity:  1,
					Price:     99.99,
				},
			},
			TotalAmount: 99.99,
			CreatedAt:   "2026-09-17T00:00:00Z",
		},
	}, nil
}

func (m *mockOrderClient) CreateOrder(ctx context.Context, in *orderpb.CreateOrderRequest, opts ...grpc.CallOption) (*orderpb.CreateOrderResponse, error) {
	return &orderpb.CreateOrderResponse{
		Order: &orderpb.Order{
			Id:          "ord-new",
			UserId:      in.UserId,
			Status:      "CREATED",
			TotalAmount: 150.00,
			CreatedAt:   "2026-09-17T00:00:00Z",
		},
	}, nil
}

// ----------------------------------------------------------------------------
// Helper to Setup Test Fiber App
// ----------------------------------------------------------------------------

func setupTestApp(introEnabled bool) *fiber.App {
	clients := gatewayGRPC.NewClientsWithServices(
		&mockUserClient{},
		&mockProductClient{},
		&mockCartClient{},
		&mockOrderClient{},
	)

	resolver := NewResolver(clients)
	schema, _ := NewSchema(resolver)

	cfg := &config.Config{
		GraphQLIntrospectionEnabled: introEnabled,
	}

	handler := NewHandler(schema, cfg)

	app := fiber.New()
	handler.RegisterRoutes(app)
	return app
}

// ----------------------------------------------------------------------------
// Test Cases
// ----------------------------------------------------------------------------

func TestHandleQuery_ValidProductQuery(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `query { product(id: "prod-100") { id name price description } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data, ok := res["data"].(map[string]interface{})
	if !ok || data == nil {
		t.Fatalf("expected data field in response, got %v", res)
	}

	product, ok := data["product"].(map[string]interface{})
	if !ok || product == nil {
		t.Fatalf("expected product object in data, got %v", data)
	}

	if product["id"] != "prod-100" {
		t.Errorf("expected product id 'prod-100', got '%v'", product["id"])
	}
	if product["name"] != "Mock Laptop" {
		t.Errorf("expected product name 'Mock Laptop', got '%v'", product["name"])
	}
}

func TestHandleQuery_ValidProductsListQuery(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `query { products(limit: 5) { id name price } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data := res["data"].(map[string]interface{})
	products := data["products"].([]interface{})
	if len(products) != 2 {
		t.Errorf("expected 2 products, got %d", len(products))
	}
}

func TestHandleMutation_Login(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `mutation { login(input: {email: "admin@gocart.com", password: "password123"}) { token user { id email } } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data := res["data"].(map[string]interface{})
	login := data["login"].(map[string]interface{})

	if login["token"] != "jwt-mock-token" {
		t.Errorf("expected token 'jwt-mock-token', got '%v'", login["token"])
	}
}

func TestHandleMutation_CreateProduct(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `mutation { createProduct(input: {name: "Wireless Mouse", price: 49.99, stockQuantity: 100}) { id name price } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data := res["data"].(map[string]interface{})
	cp := data["createProduct"].(map[string]interface{})

	if cp["id"] != "p-new" {
		t.Errorf("expected id 'p-new', got '%v'", cp["id"])
	}
	if cp["name"] != "Wireless Mouse" {
		t.Errorf("expected name 'Wireless Mouse', got '%v'", cp["name"])
	}
}

func TestHandleMutation_AddToCart(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `mutation { addToCart(input: {userId: "u-1", productId: "p-10", quantity: 3}) { id userId totalAmount } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data := res["data"].(map[string]interface{})
	cart := data["addToCart"].(map[string]interface{})

	if cart["userId"] != "u-1" {
		t.Errorf("expected userId 'u-1', got '%v'", cart["userId"])
	}
	if cart["totalAmount"] != 75.0 {
		t.Errorf("expected totalAmount 75.0, got '%v'", cart["totalAmount"])
	}
}

func TestHandleQuery_InvalidGraphQLSyntax(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `query { invalidField { `,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	errors, ok := res["errors"].([]interface{})
	if !ok || len(errors) == 0 {
		t.Fatalf("expected errors array for syntax error, got %v", res)
	}
}

func TestHandleQuery_IntrospectionEnabled(t *testing.T) {
	app := setupTestApp(true)

	reqBody := map[string]interface{}{
		"query": `query { __schema { queryType { name } } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	data, ok := res["data"].(map[string]interface{})
	if !ok || data == nil {
		t.Fatalf("expected data for enabled introspection, got %v", res)
	}
}

func TestHandleQuery_IntrospectionDisabled(t *testing.T) {
	app := setupTestApp(false) // introspection disabled

	reqBody := map[string]interface{}{
		"query": `query { __schema { queryType { name } } }`,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	errs, ok := res["errors"].([]interface{})
	if !ok || len(errs) == 0 {
		t.Fatalf("expected error when introspection disabled, got %v", res)
	}

	firstErr := errs[0].(map[string]interface{})
	if firstErr["message"] != "GraphQL introspection is disabled" {
		t.Errorf("expected message 'GraphQL introspection is disabled', got '%v'", firstErr["message"])
	}
}

func TestHandlePlayground_Enabled(t *testing.T) {
	app := setupTestApp(true)

	req := httptest.NewRequest(http.MethodGet, "/playground", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandlePlayground_Disabled(t *testing.T) {
	app := setupTestApp(false)

	req := httptest.NewRequest(http.MethodGet, "/playground", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", resp.StatusCode)
	}
}

