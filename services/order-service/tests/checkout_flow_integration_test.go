package tests

import (
	"context"
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

	svc := service.NewOrderService(repo, cartClient, prodClient, invClient, 5, nil)
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

	// 3. Verify order transitioned to CONFIRMED and PAID
	updatedOrder, err := repo.GetOrderByID(ctx, createdOrder.GetId())
	if err != nil {
		t.Fatalf("GetOrderByID failed: %v", err)
	}
	if updatedOrder.Status != model.OrderStatusConfirmed {
		t.Errorf("expected order status CONFIRMED, got: %s", updatedOrder.Status)
	}
	if updatedOrder.PaymentStatus != model.PaymentStatusPaid {
		t.Errorf("expected payment status PAID, got: %s", updatedOrder.PaymentStatus)
	}

	// Verify OrderConfirmed outbox event was generated
	repo.mu.RLock()
	var orderConfirmedEnv *events.EventEnvelope
	outboxCountBeforeDup := len(repo.outbox)
	for _, env := range repo.outbox {
		if env.EventType == events.EventTypeOrderConfirmed {
			orderConfirmedEnv = env
			break
		}
	}
	repo.mu.RUnlock()

	if orderConfirmedEnv == nil {
		t.Fatal("expected OrderConfirmed event in outbox, none found")
	}
	if orderConfirmedEnv.CorrelationID != corrID {
		t.Errorf("expected correlation ID %s on OrderConfirmed event, got %s", corrID, orderConfirmedEnv.CorrelationID)
	}

	// 4. Test Idempotency: duplicate PaymentSuccessful event
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
