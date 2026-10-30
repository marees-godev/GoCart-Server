package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/repository"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	OrderCreatedTopic       = events.TopicOrderCreated
	OrderCancelledTopic     = events.TopicOrderCancelled
	OrderConfirmedTopic     = events.TopicOrderConfirmed
	OrderPaymentFailedTopic = events.TopicOrderPaymentFailed
)

type OrderService interface {
	CreateOrder(ctx context.Context, input dto.CreateOrderInput) (*dto.CreateOrderResult, error)
	GetOrder(ctx context.Context, id string, userID ...string) (*model.Order, error)
	ListOrders(ctx context.Context, input dto.ListOrdersInput) (*dto.ListOrdersResult, error)
	UpdateOrderStatus(ctx context.Context, id string, orderStatus model.OrderStatus, reason ...string) (*model.Order, error)
	CancelOrder(ctx context.Context, id string, userID string, reason string) (*model.Order, error)
	GetOrderStatusHistory(ctx context.Context, orderID string) ([]*model.OrderStatusHistory, error)
	HandlePaymentSuccessful(ctx context.Context, event *events.PaymentSuccessfulEvent, correlationID, traceID string) (*model.Order, error)
	HandlePaymentFailed(ctx context.Context, event *events.PaymentFailedEvent, correlationID, traceID string) (*model.Order, error)
}

type orderService struct {
	repo            repository.OrderRepository
	cartClient      client.CartClient
	productClient   client.ProductClient
	inventoryClient client.InventoryClient
	clientTimeout   time.Duration
	log             *slog.Logger
}

func NewOrderService(
	repo repository.OrderRepository,
	cartClient client.CartClient,
	productClient client.ProductClient,
	inventoryClient client.InventoryClient,
	timeoutSeconds int,
	log *slog.Logger,
) OrderService {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}
	if log == nil {
		log = slog.Default()
	}
	return &orderService{
		repo:            repo,
		cartClient:      cartClient,
		productClient:   productClient,
		inventoryClient: inventoryClient,
		clientTimeout:   time.Duration(timeoutSeconds) * time.Second,
		log:             log,
	}
}

type checkoutItem struct {
	ProductID string
	StoreID   string
	Quantity  int
}

func (s *orderService) CreateOrder(ctx context.Context, input dto.CreateOrderInput) (*dto.CreateOrderResult, error) {
	if strings.TrimSpace(input.UserID) == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}
	if _, err := uuid.Parse(strings.TrimSpace(input.UserID)); err != nil {
		return nil, appErrors.BadRequest("invalid user_id format: must be valid UUID")
	}

	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return nil, appErrors.BadRequest("idempotency_key is required")
	}

	// Check if orders for this idempotency key already exist
	existingOrders, err := s.repo.GetOrdersByIdempotencyKey(ctx, strings.TrimSpace(input.IdempotencyKey))
	if err != nil && !errorsIs(err, repository.ErrOrderNotFound) {
		return nil, appErrors.Internal(err, "failed to check idempotency key")
	}
	if len(existingOrders) > 0 {
		first := existingOrders[0]
		if first.UserID != strings.TrimSpace(input.UserID) {
			return nil, appErrors.Conflict("idempotency key already used for another user")
		}
		parentID := first.ID
		if first.ParentOrderID != nil && *first.ParentOrderID != "" {
			parentID = *first.ParentOrderID
		}
		s.log.Info("Returning existing order(s) for duplicate idempotency key",
			"idempotency_key", input.IdempotencyKey,
			"order_count", len(existingOrders),
		)
		return &dto.CreateOrderResult{
			PrimaryOrder:  first,
			Orders:        existingOrders,
			ParentOrderID: parentID,
		}, nil
	}

	if strings.TrimSpace(input.ShippingAddress) == "" && strings.TrimSpace(input.ShippingAddressID) == "" {
		return nil, appErrors.BadRequest("shipping address or shipping address id is required")
	}
	if input.ShippingAddressID != "" {
		if _, err := uuid.Parse(strings.TrimSpace(input.ShippingAddressID)); err != nil {
			return nil, appErrors.BadRequest("invalid shipping_address_id format: must be valid UUID")
		}
	}
	if input.BillingAddressID != "" {
		if _, err := uuid.Parse(strings.TrimSpace(input.BillingAddressID)); err != nil {
			return nil, appErrors.BadRequest("invalid billing_address_id format: must be valid UUID")
		}
	}

	// 1. Gather items from Cart Service or input items
	rawItems, err := s.fetchCheckoutItems(ctx, input)
	if err != nil {
		return nil, err
	}
	if len(rawItems) == 0 {
		return nil, appErrors.BadRequest("order cannot be created from an empty or invalid cart")
	}

	// 2. Validate product availability and authoritative pricing per item
	type enrichedItem struct {
		productID   string
		storeID     string
		sku         string
		productName string
		quantity    int
		unitPrice   float64
		tax         float64
		discount    float64
		totalPrice  float64
	}

	storeMap := make(map[string][]enrichedItem)

	for _, item := range rawItems {
		if item.Quantity <= 0 {
			return nil, appErrors.BadRequest(fmt.Sprintf("invalid quantity %d for product %s", item.Quantity, item.ProductID))
		}
		if _, err := uuid.Parse(strings.TrimSpace(item.ProductID)); err != nil {
			return nil, appErrors.BadRequest(fmt.Sprintf("invalid product_id format: %s", item.ProductID))
		}

		// 2a. Revalidate stock availability with Inventory Service
		if s.inventoryClient != nil {
			callCtx, cancel := context.WithTimeout(ctx, s.clientTimeout)
			stock, invErr := s.inventoryClient.GetStock(callCtx, item.ProductID)
			cancel()

			if invErr != nil {
				st, ok := status.FromError(invErr)
				if ok && st.Code() == codes.NotFound {
					return nil, appErrors.Conflict(fmt.Sprintf("product %s is out of stock (not found in inventory)", item.ProductID))
				}
				return nil, appErrors.Internal(invErr, fmt.Sprintf("failed to verify stock for product %s", item.ProductID))
			}

			if stock == nil || stock.GetAvailableQuantity() < int32(item.Quantity) {
				avail := int32(0)
				if stock != nil {
					avail = stock.GetAvailableQuantity()
				}
				return nil, appErrors.Conflict(fmt.Sprintf("insufficient stock for product %s: available %d, requested %d", item.ProductID, avail, item.Quantity))
			}
		}

		// 2b. Validate product existence, status, and authoritative pricing with Product Service
		unitPrice := 0.0
		sku := fmt.Sprintf("SKU-%s", item.ProductID[:8])
		productName := "Product " + item.ProductID
		storeID := item.StoreID
		var taxRate float64

		if s.productClient != nil {
			callCtx, cancel := context.WithTimeout(ctx, s.clientTimeout)
			product, prodErr := s.productClient.GetProduct(callCtx, item.ProductID)
			cancel()

			if prodErr != nil {
				st, ok := status.FromError(prodErr)
				if ok && st.Code() == codes.NotFound {
					return nil, appErrors.NotFound(fmt.Sprintf("product %s not found", item.ProductID))
				}
				return nil, appErrors.Internal(prodErr, fmt.Sprintf("failed to fetch product %s from product service", item.ProductID))
			}
			if product == nil {
				return nil, appErrors.NotFound(fmt.Sprintf("product %s not found", item.ProductID))
			}

			prodStatus := strings.ToUpper(strings.TrimSpace(product.GetStatus()))
			if prodStatus != "PUBLISHED" && prodStatus != "IN_STOCK" && prodStatus != "ACTIVE" {
				return nil, appErrors.BadRequest(fmt.Sprintf("product %s is currently unavailable (status: %s)", item.ProductID, product.GetStatus()))
			}

			// Authoritative price snapshot
			unitPrice = product.GetPrice()
			if product.GetSku() != "" {
				sku = product.GetSku()
			}
			if product.GetName() != "" {
				productName = product.GetName()
			}
			if product.GetStoreId() != "" {
				storeID = product.GetStoreId()
			}
			taxRate = product.GetTax()
		}

		if unitPrice < 0 {
			return nil, appErrors.BadRequest(fmt.Sprintf("invalid authoritative price for product %s", item.ProductID))
		}

		if strings.TrimSpace(storeID) == "" {
			storeID = "default-store"
		}

		itemSubtotal := unitPrice * float64(item.Quantity)
		var itemTax float64
		if taxRate > 1.0 {
			itemTax = math.Round(itemSubtotal*(taxRate/100.0)*100) / 100
		} else if taxRate > 0 {
			itemTax = math.Round(itemSubtotal*taxRate*100) / 100
		}
		itemDiscount := 0.00
		itemTotal := math.Round((itemSubtotal+itemTax-itemDiscount)*100) / 100

		storeMap[storeID] = append(storeMap[storeID], enrichedItem{
			productID:   item.ProductID,
			storeID:     storeID,
			sku:         sku,
			productName: productName,
			quantity:    item.Quantity,
			unitPrice:   unitPrice,
			tax:         itemTax,
			discount:    itemDiscount,
			totalPrice:  itemTotal,
		})
	}

	// 3. Common parent_order_id across all store orders resulting from this checkout
	parentOrderID := uuid.New().String()
	now := time.Now().UTC()
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	switch currency {
	case "", "USD":
		currency = "USD"
	case "IND-RUPEE", "INDIAN-RUPEE", "RUPEE", "RUPEES":
		currency = "INR"
	default:
		if len(currency) != 3 {
			return nil, appErrors.BadRequest(fmt.Sprintf("invalid currency %q: must be a 3-letter ISO 4217 code (e.g. INR, USD)", input.Currency))
		}
	}

	orders := make([]*model.Order, 0, len(storeMap))
	envelopes := make([]*events.EventEnvelope, 0, len(storeMap))

	// Split store shipping fee and discount evenly if provided at order level
	numStores := float64(len(storeMap))
	storeShippingFee := 0.0
	if input.ShippingFee > 0 {
		storeShippingFee = math.Round((input.ShippingFee/numStores)*100) / 100
	}
	storeDiscount := 0.0
	if input.DiscountAmount > 0 {
		storeDiscount = math.Round((input.DiscountAmount/numStores)*100) / 100
	}

	firstStore := true
	for storeID, items := range storeMap {
		orderID := uuid.New().String()
		orderNumber := generateOrderNumber()

		var subtotal float64
		var taxAmount float64
		orderItems := make([]model.OrderItem, 0, len(items))
		itemPayloads := make([]events.OrderItemPayload, 0, len(items))

		for _, it := range items {
			subtotal += it.unitPrice * float64(it.quantity)
			taxAmount += it.tax

			orderItems = append(orderItems, model.OrderItem{
				ID:          uuid.New().String(),
				OrderID:     orderID,
				ProductID:   it.productID,
				StoreID:     storeID,
				SKU:         it.sku,
				ProductName: it.productName,
				Quantity:    it.quantity,
				UnitPrice:   it.unitPrice,
				Tax:         it.tax,
				Discount:    it.discount,
				TotalPrice:  it.totalPrice,
			})

			itemPayloads = append(itemPayloads, events.OrderItemPayload{
				ProductID:   it.productID,
				StoreID:     storeID,
				SKU:         it.sku,
				ProductName: it.productName,
				Quantity:    it.quantity,
				UnitPrice:   it.unitPrice,
				Tax:         it.tax,
				Discount:    it.discount,
				TotalPrice:  it.totalPrice,
			})
		}

		subtotal = math.Round(subtotal*100) / 100
		taxAmount = math.Round(taxAmount*100) / 100
		totalAmount := math.Round((subtotal+taxAmount+storeShippingFee-storeDiscount)*100) / 100
		if totalAmount < 0 {
			totalAmount = 0
		}

		var parentIDPtr *string = &parentOrderID
		var shipAddrIDPtr *string
		if input.ShippingAddressID != "" {
			shipAddrIDPtr = &input.ShippingAddressID
		}
		var billAddrIDPtr *string
		if input.BillingAddressID != "" {
			billAddrIDPtr = &input.BillingAddressID
		}
		var idempKeyPtr *string
		if firstStore && strings.TrimSpace(input.IdempotencyKey) != "" {
			k := strings.TrimSpace(input.IdempotencyKey)
			idempKeyPtr = &k
			firstStore = false
		}

		ord := &model.Order{
			ID:                orderID,
			OrderNumber:       orderNumber,
			ParentOrderID:     parentIDPtr,
			UserID:            input.UserID,
			StoreID:           storeID,
			Subtotal:          subtotal,
			TaxAmount:         taxAmount,
			ShippingFee:       storeShippingFee,
			DiscountAmount:    storeDiscount,
			TotalAmount:       totalAmount,
			Currency:          currency,
			PaymentStatus:     model.PaymentStatusPending,
			Status:            model.OrderStatusCreated,
			ShippingAddress:   input.ShippingAddress,
			ShippingAddressID: shipAddrIDPtr,
			BillingAddressID:  billAddrIDPtr,
			IdempotencyKey:    idempKeyPtr,
			Items:             orderItems,
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		createdEvt := &events.OrderCreatedEvent{
			OrderID:           ord.ID,
			ParentOrderID:     parentOrderID,
			StoreID:           ord.StoreID,
			UserID:            ord.UserID,
			Subtotal:          ord.Subtotal,
			TaxAmount:         ord.TaxAmount,
			ShippingFee:       ord.ShippingFee,
			DiscountAmount:    ord.DiscountAmount,
			TotalAmount:       ord.TotalAmount,
			Currency:          ord.Currency,
			PaymentStatus:     string(ord.PaymentStatus),
			Status:            string(ord.Status),
			OrderStatus:       string(ord.Status),
			ShippingAddressID: input.ShippingAddressID,
			BillingAddressID:  input.BillingAddressID,
			Items:             itemPayloads,
			CreatedAt:         now,
		}

		envelope, err := events.NewEventEnvelopeWithAggregate(
			events.EventTypeOrderCreated,
			"order-service",
			ord.ID,
			createdEvt,
		)
		if err != nil {
			return nil, appErrors.Internal(err, "failed to create event envelope")
		}

		if corrID, ok := ctx.Value(logger.CorrelationIDKey).(string); ok && corrID != "" {
			envelope.CorrelationID = corrID
		} else if reqID, ok := ctx.Value(logger.RequestIDKey).(string); ok && reqID != "" {
			envelope.CorrelationID = reqID
		} else {
			envelope.CorrelationID = ord.ID
		}
		span := trace.SpanFromContext(ctx)
		if span.SpanContext().IsValid() {
			envelope.TraceID = span.SpanContext().TraceID().String()
		}

		orders = append(orders, ord)
		envelopes = append(envelopes, envelope)
	}

	// 4. Persist orders and transactional outbox events in a single database transaction
	if err := s.repo.CreateOrdersWithOutbox(ctx, orders, envelopes, OrderCreatedTopic); err != nil {
		s.log.Error("Failed to persist orders with outbox", "error", err, "user_id", input.UserID)
		return nil, appErrors.Internal(err, "failed to create order")
	}

	// 5. Clean up shopping cart after successful checkout
	if s.cartClient != nil {
		clearCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.cartClient.ClearCart(clearCtx, input.UserID); err != nil {
			s.log.Warn("Failed to clear cart after order creation", "user_id", input.UserID, "error", err)
		}
	}

	var primaryOrder *model.Order
	if len(orders) > 0 {
		primaryOrder = orders[0]
	}

	return &dto.CreateOrderResult{
		PrimaryOrder:  primaryOrder,
		Orders:        orders,
		ParentOrderID: parentOrderID,
	}, nil
}

func (s *orderService) fetchCheckoutItems(ctx context.Context, input dto.CreateOrderInput) ([]checkoutItem, error) {
	if len(input.Items) > 0 {
		items := make([]checkoutItem, len(input.Items))
		for i, it := range input.Items {
			items[i] = checkoutItem{
				ProductID: it.ProductID,
				StoreID:   it.StoreID,
				Quantity:  it.Quantity,
			}
		}
		return items, nil
	}

	if s.cartClient != nil {
		callCtx, cancel := context.WithTimeout(ctx, s.clientTimeout)
		cart, err := s.cartClient.GetCart(callCtx, input.UserID)
		cancel()

		if err != nil {
			st, ok := status.FromError(err)
			if ok && st.Code() == codes.NotFound {
				return nil, appErrors.BadRequest("cart not found or empty")
			}
			return nil, appErrors.Internal(err, "failed to fetch user cart")
		}

		if cart == nil || len(cart.GetItems()) == 0 {
			return nil, appErrors.BadRequest("cart is empty")
		}

		items := make([]checkoutItem, 0, len(cart.GetItems()))
		for _, it := range cart.GetItems() {
			items = append(items, checkoutItem{
				ProductID: it.GetProductId(),
				StoreID:   it.GetStoreId(),
				Quantity:  int(it.GetQuantity()),
			})
		}
		return items, nil
	}

	return nil, appErrors.BadRequest("cart is empty or no items provided")
}

func (s *orderService) GetOrder(ctx context.Context, id string, userID ...string) (*model.Order, error) {
	if strings.TrimSpace(id) == "" {
		return nil, appErrors.BadRequest("order id is required")
	}
	order, err := s.repo.GetOrderByID(ctx, id)
	if err != nil {
		if errorsIs(err, repository.ErrOrderNotFound) {
			return nil, appErrors.NotFound("order not found")
		}
		return nil, appErrors.Internal(err, "failed to get order")
	}

	// Authorization check: Users can only retrieve their own orders.
	callerUserID := ""
	callerRole := ""
	if len(userID) > 0 && strings.TrimSpace(userID[0]) != "" {
		callerUserID = strings.TrimSpace(userID[0])
	}
	if userCtx, ok := auth.UserFromContext(ctx); ok && userCtx != nil {
		if callerUserID == "" {
			callerUserID = userCtx.UserID
		}
		callerRole = userCtx.Role
	}

	if callerRole != "ADMIN" && callerRole != auth.RoleAdmin && callerRole != "DELIVERY_PARTNER" && callerRole != auth.RoleDeliveryPartner && callerRole != "MERCHANT" && callerRole != auth.RoleMerchant {
		if callerUserID != "" && order.UserID != callerUserID {
			return nil, appErrors.Forbidden("you do not have permission to view this order")
		}
	}

	return order, nil
}

func (s *orderService) ListOrders(ctx context.Context, input dto.ListOrdersInput) (*dto.ListOrdersResult, error) {
	if strings.TrimSpace(input.UserID) == "" {
		return nil, appErrors.BadRequest("user_id is required")
	}
	orders, total, err := s.repo.ListOrdersByUserID(ctx, input.UserID, input.Limit, input.Offset)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to list orders")
	}
	return &dto.ListOrdersResult{
		Orders: orders,
		Total:  total,
	}, nil
}

func (s *orderService) UpdateOrderStatus(ctx context.Context, id string, orderStatus model.OrderStatus, reason ...string) (*model.Order, error) {
	if strings.TrimSpace(id) == "" {
		return nil, appErrors.BadRequest("order id is required")
	}
	if !orderStatus.IsValid() {
		return nil, appErrors.BadRequest(fmt.Sprintf("invalid order status: %s", orderStatus))
	}

	order, err := s.repo.GetOrderByID(ctx, id)
	if err != nil {
		if errorsIs(err, repository.ErrOrderNotFound) {
			return nil, appErrors.NotFound("order not found")
		}
		return nil, appErrors.Internal(err, "failed to get order")
	}

	if order.Status == orderStatus {
		return order, nil
	}

	if !model.IsValidTransition(order.Status, orderStatus) {
		return nil, appErrors.BadRequest(fmt.Sprintf("invalid order status transition from %s to %s", order.Status, orderStatus))
	}

	updatedOrder, err := s.repo.UpdateOrderStatus(ctx, id, orderStatus, reason...)
	if err != nil {
		if errorsIs(err, repository.ErrOrderNotFound) {
			return nil, appErrors.NotFound("order not found")
		}
		return nil, err
	}
	return updatedOrder, nil
}

func (s *orderService) HandlePaymentSuccessful(ctx context.Context, event *events.PaymentSuccessfulEvent, correlationID, traceID string) (*model.Order, error) {
	if event == nil || strings.TrimSpace(event.OrderID) == "" {
		return nil, appErrors.BadRequest("order_id is required in PaymentSuccessful payload")
	}

	orderID := strings.TrimSpace(event.OrderID)
	order, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		if errorsIs(err, repository.ErrOrderNotFound) {
			return nil, appErrors.NotFound("order not found for payment success")
		}
		return nil, appErrors.Internal(err, "failed to get order")
	}

	// Idempotency: if already confirmed or paid, return early without state changes
	if order.Status == model.OrderStatusConfirmed || order.Status == model.OrderStatusPaid {
		s.log.Info("Order already confirmed/paid, ignoring duplicate PaymentSuccessful event",
			"order_id", orderID,
			"current_status", order.Status,
		)
		return order, nil
	}

	now := time.Now().UTC()
	confirmedEvt := events.OrderConfirmedEvent{
		OrderID:     order.ID,
		ConfirmedAt: now,
	}

	envelope, err := events.NewEventEnvelopeWithAggregate(
		events.EventTypeOrderConfirmed,
		"order-service",
		order.ID,
		confirmedEvt,
	)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to create OrderConfirmed event envelope")
	}
	if correlationID != "" {
		envelope.CorrelationID = correlationID
	}
	if traceID != "" {
		envelope.TraceID = traceID
	}

	updatedOrder, err := s.repo.UpdateOrderStatusWithOutbox(
		ctx,
		orderID,
		model.OrderStatusConfirmed,
		envelope,
		events.TopicOrderConfirmed,
		"Payment successful",
	)
	if err != nil {
		return nil, err
	}

	s.log.Info("Order transitioned to CONFIRMED and OrderConfirmed event recorded in outbox",
		"order_id", orderID,
		"payment_id", event.PaymentID,
	)
	return updatedOrder, nil
}

func (s *orderService) HandlePaymentFailed(ctx context.Context, event *events.PaymentFailedEvent, correlationID, traceID string) (*model.Order, error) {
	if event == nil || strings.TrimSpace(event.OrderID) == "" {
		return nil, appErrors.BadRequest("order_id is required in PaymentFailed payload")
	}

	orderID := strings.TrimSpace(event.OrderID)
	order, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		if errorsIs(err, repository.ErrOrderNotFound) {
			return nil, appErrors.NotFound("order not found for payment failed")
		}
		return nil, appErrors.Internal(err, "failed to get order")
	}

	// Idempotency: if already payment failed, return early
	if order.Status == model.OrderStatusPaymentFailed {
		s.log.Info("Order already marked as PAYMENT_FAILED, ignoring duplicate event",
			"order_id", orderID,
		)
		return order, nil
	}

	reason := event.Reason
	if reason == "" {
		reason = "Payment failed at gateway"
	}

	now := time.Now().UTC()
	paymentFailedEvt := events.OrderPaymentFailedEvent{
		OrderID:   order.ID,
		PaymentID: event.PaymentID,
		Reason:    reason,
		FailedAt:  now,
	}

	envelope, err := events.NewEventEnvelopeWithAggregate(
		events.EventTypeOrderPaymentFailed,
		"order-service",
		order.ID,
		paymentFailedEvt,
	)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to create OrderPaymentFailed event envelope")
	}
	if correlationID != "" {
		envelope.CorrelationID = correlationID
	}
	if traceID != "" {
		envelope.TraceID = traceID
	}

	updatedOrder, err := s.repo.UpdateOrderStatusWithOutbox(
		ctx,
		orderID,
		model.OrderStatusPaymentFailed,
		envelope,
		OrderPaymentFailedTopic,
		reason,
	)
	if err != nil {
		return nil, err
	}

	s.log.Info("Order transitioned to PAYMENT_FAILED and OrderPaymentFailed event recorded in outbox",
		"order_id", orderID,
		"reason", reason,
	)
	return updatedOrder, nil
}

func (s *orderService) CancelOrder(ctx context.Context, id string, userID string, reason string) (*model.Order, error) {
	if strings.TrimSpace(id) == "" {
		return nil, appErrors.BadRequest("order id is required")
	}

	order, err := s.repo.GetOrderByID(ctx, id)
	if err != nil {
		if errorsIs(err, repository.ErrOrderNotFound) {
			return nil, appErrors.NotFound("order not found")
		}
		return nil, appErrors.Internal(err, "failed to get order")
	}

	// 1. Authorization check: Cancellation authorization is enforced by Order Service.
	callerUserID := strings.TrimSpace(userID)
	callerRole := ""
	if userCtx, ok := auth.UserFromContext(ctx); ok && userCtx != nil {
		if callerUserID == "" {
			callerUserID = userCtx.UserID
		}
		callerRole = userCtx.Role
	}

	if callerRole != "ADMIN" && callerRole != auth.RoleAdmin {
		if callerUserID == "" || order.UserID != callerUserID {
			return nil, appErrors.Forbidden("you do not have permission to cancel this order")
		}
	}

	// 2. State transition check:
	if order.Status == model.OrderStatusCancelled {
		return nil, appErrors.BadRequest("order is already cancelled")
	}
	if !model.CanCancel(order.Status) {
		return nil, appErrors.BadRequest(fmt.Sprintf("cannot cancel order in %s status", order.Status))
	}

	// 3. Prepare outbox envelope if supported
	now := time.Now().UTC()
	var envelope *events.EventEnvelope
	eventPayload := events.OrderCancelledEvent{
		OrderID:     order.ID,
		Reason:      reason,
		CancelledAt: now,
	}
	env, envErr := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderCancelled, "order-service", order.ID, eventPayload)
	if envErr == nil {
		envelope = env
	}

	updatedOrder, err := s.repo.CancelOrder(ctx, id, envelope, OrderCancelledTopic, reason)
	if err != nil {
		if errorsIs(err, repository.ErrOrderNotFound) {
			return nil, appErrors.NotFound("order not found")
		}
		return nil, err
	}

	return updatedOrder, nil
}

func (s *orderService) GetOrderStatusHistory(ctx context.Context, orderID string) ([]*model.OrderStatusHistory, error) {
	if strings.TrimSpace(orderID) == "" {
		return nil, appErrors.BadRequest("order id is required")
	}

	if _, err := s.repo.GetOrderByID(ctx, orderID); err != nil {
		if errorsIs(err, repository.ErrOrderNotFound) {
			return nil, appErrors.NotFound("order not found")
		}
		return nil, appErrors.Internal(err, "failed to get order")
	}

	history, err := s.repo.GetOrderStatusHistory(ctx, orderID)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to get order status history")
	}

	return history, nil
}

func generateOrderNumber() string {
	datePart := time.Now().UTC().Format("20060102")
	n, _ := rand.Int(rand.Reader, big.NewInt(900000))
	randomPart := n.Int64() + 100000
	return fmt.Sprintf("ORD-%s-%06d", datePart, randomPart)
}

func errorsIs(err, target error) bool {
	if err == nil {
		return target == nil
	}
	return strings.Contains(err.Error(), target.Error())
}

var _ = cartpb.CartItem{}