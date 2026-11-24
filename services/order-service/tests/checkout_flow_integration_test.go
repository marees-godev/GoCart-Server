package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	inventorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/service"
	"google.golang.org/grpc/metadata"
)

func setupCheckoutIntegration(t *testing.T) (orderpb.OrderServiceClient, *consumer.OrderEventConsumer, *inMemOrderRepo, string, string, string, func()) {
	repo := newInMemOrderRepo()

	userID := uuid.New().String()
	storeID := uuid.New().String()
	prodID := uuid.New().String()

	cartClient := &inMemCartClient{
		carts: map[string]*cartpb.Cart{
			userID: {
				Id:     "cart-" + userID,
				UserId: userID,
				Items: []*cartpb.CartItem{
					{ProductId: prodID, StoreId: storeID, Quantity: 2, UnitPrice: 50.0},
				},
			},
		},
	}

	prodClient := &inMemProductClient{
		products: map[string]*productpb.Product{
			prodID: {
				Id:      prodID,
				StoreId: storeID,
				Sku:     "SKU-TEST-1",
				Name:    "Test Item",
				Price:   50.00,
				Tax:     0.0,
				Status:  "PUBLISHED",
			},
		},
	}

	invClient := &inMemInventoryClient{
		stocks: map[string]*inventorypb.StockItem{
			prodID: {ProductId: prodID, AvailableQuantity: 20},
		},
	}

	svc := service.NewOrderService(repo, cartClient, prodClient, invClient, nil, 5, nil)
	orderConsumer := consumer.NewOrderEventConsumer(svc)

	grpcClient, cleanup := setupInProcessGRPC(t, repo, cartClient, prodClient, invClient)
	return grpcClient, orderConsumer, repo, userID, storeID, prodID, cleanup
}

func TestCheckoutFlow_PaymentSuccessful_EndToEnd(t *testing.T) {
	grpcClient, orderConsumer, repo, userID, _, _, cleanup := setupCheckoutIntegration(t)
	defer cleanup()

	ctx := context.Background()
	corrID := "corr-" + uuid.New().String()
	ctx = logger.WithCorrelationID(ctx, corrID)
	ctx = logger.WithRequestID(ctx, corrID)
	md := metadata.New(map[string]string{
		"x-correlation-id": corrID,
		"x-request-id":     corrID,
	})
	ctx = metadata.NewOutgoingContext(ctx, md)

	// 1. Initiate checkout via gRPC
	resp, err := grpcClient.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		UserId:          userID,
		IdempotencyKey:  uuid.New().String(),
		CartId:          "cart-" + userID,
		ShippingAddress: "123 Market St, San Francisco, CA 94105",
		ShippingFee:     5.00,
	})
	if err != nil {
		t.Fatalf("CreateOrder gRPC call failed: %v", err)
	}

	if len(resp.GetOrders()) != 1 {
		t.Fatalf("expected 1 order created, got %d", len(resp.GetOrders()))
	}
	createdOrder := resp.GetOrders()[0]

	if createdOrder.GetOrderStatus() != "CREATED" {
		t.Errorf("expected initial order status CREATED, got: %s", createdOrder.GetOrderStatus())
	}
	if createdOrder.GetPaymentStatus() != "PENDING" {
		t.Errorf("expected initial payment status PENDING, got: %s", createdOrder.GetPaymentStatus())
	}

	// Verify OrderCreated outbox event was generated
	repo.mu.RLock()
	var orderCreatedEnv *events.EventEnvelope
	for _, env := range repo.outbox {
		if env.EventType == events.EventTypeOrderCreated {
			orderCreatedEnv = env
			break
		}
	}
	repo.mu.RUnlock()

	if orderCreatedEnv == nil {
		t.Fatal("expected OrderCreated event in outbox, none found")
	}
	if orderCreatedEnv.CorrelationID != corrID {
		t.Errorf("expected correlation ID %s on outbox event, got %s", corrID, orderCreatedEnv.CorrelationID)
	}

	// 2. Deliver PaymentSuccessful event to Order Consumer
	paymentID := "pay-" + uuid.New().String()
	paySuccessPayload := events.PaymentSuccessfulEvent{
		PaymentID:     paymentID,
		OrderID:       createdOrder.GetId(),
		UserID:        userID,
		Amount:        createdOrder.GetTotalAmount(),
		Currency:      "USD",
		PaymentMethod: "CREDIT_CARD",
		PaidAt:        time.Now().UTC(),
	}

	paySuccessEnv, err := events.NewEventEnvelopeWithAggregate(
		events.EventTypePaymentSuccessful,
		"payment-service",
		paymentID,
		paySuccessPayload,
	)
	if err != nil {
		t.Fatalf("failed to create PaymentSuccessful envelope: %v", err)
	}
	paySuccessEnv.CorrelationID = corrID

	if err := orderConsumer.HandleEvent(ctx, paySuccessEnv); err != nil {
		t.Fatalf("HandleEvent for PaymentSuccessful failed: %v", err)
	}

	// 3. Verify order transitioned to PAID and PaymentStatus is PAID
	updatedOrder, err := repo.GetOrderByID(ctx, createdOrder.GetId())
	if err != nil {
		t.Fatalf("GetOrderByID failed: %v", err)
	}
	if updatedOrder.Status != model.OrderStatusPaid {
		t.Errorf("expected order status PAID, got: %s", updatedOrder.Status)
	}
	if updatedOrder.PaymentStatus != model.PaymentStatusPaid {
		t.Errorf("expected payment status PAID, got: %s", updatedOrder.PaymentStatus)
	}

	// Verify OrderPaymentConfirmed outbox event was generated
	repo.mu.RLock()
	var orderPaymentConfirmedEnv *events.EventEnvelope
	outboxCountBeforeDup := len(repo.outbox)
	for _, env := range repo.outbox {
		if env.EventType == events.EventTypeOrderPaymentConfirmed {
			orderPaymentConfirmedEnv = env
			break
		}
	}
	repo.mu.RUnlock()

	if orderPaymentConfirmedEnv == nil {
		t.Fatal("expected OrderPaymentConfirmed event in outbox, none found")
	}
	if orderPaymentConfirmedEnv.CorrelationID != corrID {
		t.Errorf("expected correlation ID %s on OrderPaymentConfirmed event, got %s", corrID, orderPaymentConfirmedEnv.CorrelationID)
	}

	// Merchant confirms the order
	confirmedOrder, err := grpcClient.UpdateOrderStatus(ctx, &orderpb.UpdateOrderStatusRequest{
		Id:     createdOrder.GetId(),
		Status: "CONFIRMED",
	})
	if err != nil {
		t.Fatalf("merchant confirm order failed: %v", err)
	}
	if confirmedOrder.GetOrder().GetStatus() != "CONFIRMED" {
		t.Errorf("expected order status CONFIRMED after merchant confirmation, got: %s", confirmedOrder.GetOrder().GetStatus())
	}

	// 4. Test Idempotency: duplicate PaymentSuccessful event
	repo.mu.RLock()
	outboxCountBeforeDup = len(repo.outbox)
	repo.mu.RUnlock()

	if err := orderConsumer.HandleEvent(ctx, paySuccessEnv); err != nil {
		t.Fatalf("duplicate PaymentSuccessful HandleEvent failed: %v", err)
	}

	repo.mu.RLock()
	outboxCountAfterDup := len(repo.outbox)
	repo.mu.RUnlock()

	if outboxCountAfterDup != outboxCountBeforeDup {
		t.Errorf("expected outbox count to remain %d, got %d", outboxCountBeforeDup, outboxCountAfterDup)
	}
}

func TestCheckoutFlow_PaymentFailed_EndToEnd(t *testing.T) {
	grpcClient, orderConsumer, repo, userID, _, _, cleanup := setupCheckoutIntegration(t)
	defer cleanup()

	ctx := context.Background()
	corrID := "corr-fail-" + uuid.New().String()
	ctx = logger.WithCorrelationID(ctx, corrID)
	ctx = logger.WithRequestID(ctx, corrID)
	md := metadata.New(map[string]string{
		"x-correlation-id": corrID,
		"x-request-id":     corrID,
	})
	ctx = metadata.NewOutgoingContext(ctx, md)

	// 1. Create order
	resp, err := grpcClient.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		UserId:          userID,
		IdempotencyKey:  uuid.New().String(),
		CartId:          "cart-" + userID,
		ShippingAddress: "456 Elm St, New York, NY 10001",
		ShippingFee:     5.00,
	})
	if err != nil {
		t.Fatalf("CreateOrder gRPC call failed: %v", err)
	}
	createdOrder := resp.GetOrders()[0]

	// 2. Deliver PaymentFailed event
	paymentID := "pay-fail-" + uuid.New().String()
	payFailedPayload := events.PaymentFailedEvent{
		PaymentID: paymentID,
		OrderID:   createdOrder.GetId(),
		UserID:    userID,
		Amount:    createdOrder.GetTotalAmount(),
		Reason:    "Insufficient funds",
		FailedAt:  time.Now().UTC(),
	}

	payFailedEnv, err := events.NewEventEnvelopeWithAggregate(
		events.EventTypePaymentFailed,
		"payment-service",
		paymentID,
		payFailedPayload,
	)
	if err != nil {
		t.Fatalf("failed to create PaymentFailed envelope: %v", err)
	}
	payFailedEnv.CorrelationID = corrID

	if err := orderConsumer.HandleEvent(ctx, payFailedEnv); err != nil {
		t.Fatalf("HandleEvent for PaymentFailed failed: %v", err)
	}

	// 3. Verify order transitioned to PAYMENT_FAILED and FAILED
	updatedOrder, err := repo.GetOrderByID(ctx, createdOrder.GetId())
	if err != nil {
		t.Fatalf("GetOrderByID failed: %v", err)
	}
	if updatedOrder.Status != model.OrderStatusPaymentFailed {
		t.Errorf("expected order status PAYMENT_FAILED, got: %s", updatedOrder.Status)
	}
	if updatedOrder.PaymentStatus != model.PaymentStatusFailed {
		t.Errorf("expected payment status FAILED, got: %s", updatedOrder.PaymentStatus)
	}

	// 4. Test Idempotency: duplicate PaymentFailed event
	if err := orderConsumer.HandleEvent(ctx, payFailedEnv); err != nil {
		t.Fatalf("duplicate PaymentFailed HandleEvent failed: %v", err)
	}

	afterDupOrder, err := repo.GetOrderByID(ctx, createdOrder.GetId())
	if err != nil {
		t.Fatalf("GetOrderByID after duplicate failed: %v", err)
	}
	if afterDupOrder.Status != model.OrderStatusPaymentFailed {
		t.Errorf("expected order status to remain PAYMENT_FAILED, got: %s", afterDupOrder.Status)
	}
}

func TestCheckoutFlow_InStock_Success(t *testing.T) {
	grpcClient, _, repo, userID, storeID, prodID, cleanup := setupCheckoutIntegration(t)
	defer cleanup()

	ctx := context.Background()
	resp, err := grpcClient.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		UserId:          userID,
		IdempotencyKey:  uuid.New().String(),
		CartId:          "cart-" + userID,
		ShippingAddress: "100 Pine St, Seattle, WA 98101",
		ShippingFee:     10.00,
	})
	if err != nil {
		t.Fatalf("expected successful order creation for in-stock product, got error: %v", err)
	}

	if len(resp.GetOrders()) != 1 {
		t.Fatalf("expected 1 order created, got %d", len(resp.GetOrders()))
	}
	ord := resp.GetOrders()[0]
	if ord.GetOrderStatus() != "CREATED" {
		t.Errorf("expected order status CREATED, got %s", ord.GetOrderStatus())
	}
	if ord.GetStoreId() != storeID {
		t.Errorf("expected store ID %s, got %s", storeID, ord.GetStoreId())
	}

	// Verify order has items matching the in-stock product
	if len(ord.GetItems()) != 1 {
		t.Fatalf("expected 1 order item, got %d", len(ord.GetItems()))
	}
	if ord.GetItems()[0].GetProductId() != prodID {
		t.Errorf("expected product ID %s, got %s", prodID, ord.GetItems()[0].GetProductId())
	}

	// Verify outbox has OrderCreated event
	repo.mu.RLock()
	defer repo.mu.RUnlock()
	var foundEvent bool
	for _, env := range repo.outbox {
		if env.EventType == events.EventTypeOrderCreated {
			foundEvent = true
			break
		}
	}
	if !foundEvent {
		t.Errorf("expected OrderCreated event in outbox for in-stock order")
	}
}

func TestCheckoutFlow_InsufficientStock_Rejected(t *testing.T) {
	repo := newInMemOrderRepo()
	userID := uuid.New().String()
	storeID := uuid.New().String()
	prodID := uuid.New().String()

	cartClient := &inMemCartClient{
		carts: map[string]*cartpb.Cart{
			userID: {
				Id:     "cart-" + userID,
				UserId: userID,
				Items: []*cartpb.CartItem{
					{ProductId: prodID, StoreId: storeID, Quantity: 10, UnitPrice: 50.0},
				},
			},
		},
	}
	prodClient := &inMemProductClient{
		products: map[string]*productpb.Product{
			prodID: {
				Id:      prodID,
				StoreId: storeID,
				Sku:     "SKU-TEST-LOW",
				Name:    "Low Stock Item",
				Price:   50.00,
				Status:  "PUBLISHED",
			},
		},
	}
	// Available stock is only 3, but cart requests 10
	invClient := &inMemInventoryClient{
		stocks: map[string]*inventorypb.StockItem{
			prodID: {ProductId: prodID, AvailableQuantity: 3},
		},
	}

	grpcClient, cleanup := setupInProcessGRPC(t, repo, cartClient, prodClient, invClient)
	defer cleanup()

	ctx := context.Background()
	_, err := grpcClient.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		UserId:          userID,
		IdempotencyKey:  uuid.New().String(),
		CartId:          "cart-" + userID,
		ShippingAddress: "100 Pine St, Seattle, WA 98101",
		ShippingFee:     10.00,
	})
	if err == nil {
		t.Fatalf("expected order creation to fail due to insufficient stock, got success")
	}
	if !strings.Contains(err.Error(), "insufficient stock") {
		t.Errorf("expected error to mention 'insufficient stock', got: %v", err)
	}
}

func TestCheckoutFlow_MissingInventory_Rejected(t *testing.T) {
	repo := newInMemOrderRepo()
	userID := uuid.New().String()
	storeID := uuid.New().String()
	missingProdID := uuid.New().String()

	cartClient := &inMemCartClient{
		carts: map[string]*cartpb.Cart{
			userID: {
				Id:     "cart-" + userID,
				UserId: userID,
				Items: []*cartpb.CartItem{
					{ProductId: missingProdID, StoreId: storeID, Quantity: 1, UnitPrice: 50.0},
				},
			},
		},
	}
	prodClient := &inMemProductClient{
		products: map[string]*productpb.Product{
			missingProdID: {
				Id:      missingProdID,
				StoreId: storeID,
				Sku:     "SKU-MISSING",
				Name:    "Missing Stock Item",
				Price:   50.00,
				Status:  "PUBLISHED",
			},
		},
	}
	// Inventory client has no stock record for this product
	invClient := &inMemInventoryClient{
		stocks: map[string]*inventorypb.StockItem{},
	}

	grpcClient, cleanup := setupInProcessGRPC(t, repo, cartClient, prodClient, invClient)
	defer cleanup()

	ctx := context.Background()
	_, err := grpcClient.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		UserId:          userID,
		IdempotencyKey:  uuid.New().String(),
		CartId:          "cart-" + userID,
		ShippingAddress: "100 Pine St, Seattle, WA 98101",
		ShippingFee:     10.00,
	})
	if err == nil {
		t.Fatalf("expected order creation to fail due to missing inventory, got success")
	}
	if !strings.Contains(err.Error(), "out of stock (not found in inventory)") {
		t.Errorf("expected error to mention 'out of stock (not found in inventory)', got: %v", err)
	}
}
