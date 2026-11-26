package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"strings"
	"time"

	"encoding/json"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
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
	OrderShippedTopic       = events.TopicOrderShipped
	OrderDeliveredTopic     = events.TopicOrderDelivered
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
	deliveryClient  client.DeliveryClient
	clientTimeout   time.Duration
	log             *slog.Logger
}

func NewOrderService(
	repo repository.OrderRepository,
	cartClient client.CartClient,
	productClient client.ProductClient,
	inventoryClient client.InventoryClient,
	deliveryClient client.DeliveryClient,
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
		deliveryClient:  deliveryClient,
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
		weight      float64
	}

	storeMap := make(map[string][]enrichedItem)
	var storeOrder []string

	for i := range rawItems {
		item := &rawItems[i]
		item.ProductID = strings.TrimSpace(item.ProductID)
		item.StoreID = strings.TrimSpace(item.StoreID)
		if item.Quantity <= 0 {
			return nil, appErrors.BadRequest(fmt.Sprintf("invalid quantity %d for product %s", item.Quantity, item.ProductID))
		}
		if _, err := uuid.Parse(item.ProductID); err != nil {
			return nil, appErrors.BadRequest(fmt.Sprintf("invalid product_id format: %s", item.ProductID))
		}

		// 2a. Revalidate stock availability with Inventory Service
		if s.inventoryClient != nil {
			callCtx, cancel := context.WithTimeout(ctx, s.clientTimeout)
			stock, invErr := s.inventoryClient.GetStock(callCtx, item.ProductID)
			cancel()

			if invErr != nil {
				s.log.Warn("Failed to verify stock with inventory service", "product_id", item.ProductID, "error", invErr)
				st, ok := status.FromError(invErr)
				if ok {
					if st.Code() == codes.NotFound {
						return nil, appErrors.Conflict(fmt.Sprintf("product %s is out of stock (not found in inventory)", item.ProductID))
					}
					if st.Code() == codes.Unavailable || st.Code() == codes.DeadlineExceeded {
						return nil, appErrors.ServiceUnavailable(fmt.Sprintf("inventory service unavailable while verifying stock for product %s", item.ProductID))
					}
					if st.Code() == codes.InvalidArgument {
						return nil, appErrors.BadRequest(fmt.Sprintf("invalid inventory request for product %s: %s", item.ProductID, st.Message()))
					}
				}
				if errors.Is(invErr, context.DeadlineExceeded) {
					return nil, appErrors.ServiceUnavailable(fmt.Sprintf("inventory service unavailable while verifying stock for product %s", item.ProductID))
				}
				return nil, appErrors.Internal(invErr, fmt.Sprintf("failed to verify stock for product %s: %v", item.ProductID, invErr))
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
		weight := 0.0
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
			weight = product.GetWeight()
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

		if _, exists := storeMap[storeID]; !exists {
			storeOrder = append(storeOrder, storeID)
		}
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
			weight:      weight,
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

	var destCity, destState, destPostalCode string
	if strings.TrimSpace(input.ShippingAddress) != "" {
		var addrPayload events.ShippingAddressPayload
		if err := json.Unmarshal([]byte(input.ShippingAddress), &addrPayload); err == nil {
			destCity = addrPayload.City
			destState = addrPayload.State
			destPostalCode = addrPayload.PostalCode
			// destCountry
		}
	}

	// 1. Compute overall order subtotal across all store items
	var overallSubtotal float64
	for _, items := range storeMap {
		for _, it := range items {
			overallSubtotal += it.unitPrice * float64(it.quantity)
		}
	}
	overallSubtotal = math.Round(overallSubtotal*100) / 100

	// 2. Fixed platform fee is ₹7.00 for all orders across the platform
	orderPlatformFee := 7.00
	if input.PlatformFee > 0 {
		orderPlatformFee = input.PlatformFee
	}

	// 3. Split store shipping fee and discount evenly if provided at order level
	numStores := float64(len(storeMap))
	baseStoreShippingFee := 0.0
	if overallSubtotal >= 500.0 {
		baseStoreShippingFee = 0.0
	} else if input.ShippingFee > 0 {
		baseStoreShippingFee = math.Round((input.ShippingFee/numStores)*100) / 100
	} else {
		baseStoreShippingFee = math.Round((40.0/numStores)*100) / 100
	}
	storeDiscount := 0.0
	if input.DiscountAmount > 0 {
		storeDiscount = math.Round((input.DiscountAmount/numStores)*100) / 100
	}

	requiresShipping := strings.TrimSpace(input.ShippingAddress) != "" || strings.TrimSpace(input.ShippingAddressID) != ""
	needsShippingCalculation := requiresShipping && input.ShippingFee <= 0
	if needsShippingCalculation && s.deliveryClient == nil {
		s.log.Error("Delivery service client is unavailable for mandatory shipping fee calculation")
		return nil, appErrors.ServiceUnavailable("delivery service unavailable while calculating shipping fee")
	}

	firstStore := true
	idempKeyFirst := true
	for _, storeID := range storeOrder {
		items := storeMap[storeID]
		storeShippingFee := baseStoreShippingFee
		if needsShippingCalculation {
			if overallSubtotal >= 500.0 {
				storeShippingFee = 0.0
			} else {
				var shipItems []*deliverypb.ShippingItem
				for _, it := range items {
					w := it.weight
					if w > 0 && w <= 50 {
						w = w * 1000.0
					}
					shipItems = append(shipItems, &deliverypb.ShippingItem{
						ProductId:   it.productID,
						WeightGrams: w,
						Quantity:    int32(it.quantity),
					})
				}
				calcCtx, cancel := context.WithTimeout(ctx, s.clientTimeout)
				feeResp, feeErr := s.deliveryClient.CalculateShippingFee(calcCtx, &deliverypb.CalculateShippingFeeRequest{
					Items:                 shipItems,
					DestinationPostalCode: destPostalCode,
					DestinationCity:       destCity,
					DestinationState:      destState,
					ShippingAddress:       input.ShippingAddress,
				})
				cancel()
				if feeErr != nil {
					s.log.Error("Failed to calculate shipping fee from delivery service", "store_id", storeID, "error", feeErr)
					var appErr *appErrors.AppError
					if errors.As(feeErr, &appErr) && appErr.Code == appErrors.CodeServiceUnavailable {
						return nil, appErrors.ServiceUnavailable("delivery service unavailable while calculating shipping fee")
					}
					st, ok := status.FromError(feeErr)
					if (ok && (st.Code() == codes.Unavailable || st.Code() == codes.DeadlineExceeded)) || errors.Is(feeErr, context.DeadlineExceeded) {
						return nil, appErrors.ServiceUnavailable("delivery service unavailable while calculating shipping fee")
					}
					return nil, appErrors.Internal(feeErr, "failed to calculate shipping fee from delivery service")
				}
				if feeResp == nil {
					s.log.Error("Delivery service returned empty shipping fee response", "store_id", storeID)
					return nil, appErrors.Internal(nil, "delivery service returned an empty shipping fee response")
				}
				if feeResp.ShippingFee < 0 {
					s.log.Error("Delivery service returned negative shipping fee", "store_id", storeID, "shipping_fee", feeResp.ShippingFee)
					return nil, appErrors.Internal(nil, "delivery service returned an invalid negative shipping fee")
				}
				// Platform policy strictly charges ₹40 (split across stores) for orders below ₹500
				storeShippingFee = baseStoreShippingFee
			}
		}
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

		// Apply platform fee once at the primary order level for multi-store orders
		storePlatformFee := 0.0
		if firstStore {
			storePlatformFee = orderPlatformFee
			firstStore = false
		}

		totalAmount := math.Round((subtotal+taxAmount+storePlatformFee+storeShippingFee-storeDiscount)*100) / 100
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
		if idempKeyFirst && strings.TrimSpace(input.IdempotencyKey) != "" {
			k := strings.TrimSpace(input.IdempotencyKey)
			idempKeyPtr = &k
			idempKeyFirst = false
		}

		ord := &model.Order{
			ID:                orderID,
			OrderNumber:       orderNumber,
			ParentOrderID:     parentIDPtr,
			UserID:            input.UserID,
			StoreID:           storeID,
			Subtotal:          subtotal,
			TaxAmount:         taxAmount,
			PlatformFee:       storePlatformFee,
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
			PlatformFee:       ord.PlatformFee,
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
				ProductID: strings.TrimSpace(it.ProductID),
				StoreID:   strings.TrimSpace(it.StoreID),
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
				ProductID: strings.TrimSpace(it.GetProductId()),
				StoreID:   strings.TrimSpace(it.GetStoreId()),
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
		callerRole = strings.ToUpper(strings.TrimSpace(userCtx.Role))
	}

	// If no caller identity context is provided, allow internal service query.
	if callerUserID == "" && callerRole == "" {
		return order, nil
	}

	// 1. If caller is the user who placed the order, allow access regardless of role (CUSTOMER, MERCHANT, DELIVERY_PARTNER).
	if callerUserID != "" && order.UserID == callerUserID {
		return order, nil
	}

	// 2. If caller is an ADMIN, allow viewing any order.
	if callerRole == auth.RoleAdmin {
		return order, nil
	}

	// 3. If caller is a MERCHANT, only allow if this order belongs to one of the merchant's stores / items.
	if callerRole == auth.RoleMerchant {
		if order.StoreID != "" && order.StoreID == callerUserID {
			return order, nil
		}
		for _, it := range order.Items {
			if it.StoreID != "" && it.StoreID == callerUserID {
				return order, nil
			}
		}
	}

	return nil, appErrors.Forbidden("you do not have permission to view this order")
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

	if orderStatus == model.OrderStatusConfirmed {
		if order.PaymentStatus != model.PaymentStatusPaid {
			return nil, appErrors.FailedPrecondition(fmt.Sprintf("cannot confirm order: payment status must be PAID (current payment status: %s)", order.PaymentStatus))
		}
	}

	now := time.Now().UTC()
	var envelope *events.EventEnvelope
	var topic string
	reasonStr := ""
	if len(reason) > 0 {
		reasonStr = reason[0]
	}

	switch orderStatus {
	case model.OrderStatusPaid:
		topic = events.TopicOrderPaymentConfirmed
		payload := events.OrderPaymentConfirmedEvent{
			OrderID:       order.ID,
			OrderNumber:   order.OrderNumber,
			UserID:        order.UserID,
			StoreID:       order.StoreID,
			TotalAmount:   order.TotalAmount,
			Currency:      order.Currency,
			PaymentStatus: string(model.PaymentStatusPaid),
			PaidAt:        now,
		}
		envelope, _ = events.NewEventEnvelopeWithAggregate(events.EventTypeOrderPaymentConfirmed, "order-service", order.ID, payload)
	case model.OrderStatusConfirmed:
		topic = OrderConfirmedTopic
		payload := events.OrderConfirmedEvent{
			OrderID:       order.ID,
			UserID:        order.UserID,
			PaymentStatus: string(model.PaymentStatusPaid),
			ConfirmedAt:   now,
		}
		envelope, _ = events.NewEventEnvelopeWithAggregate(events.EventTypeOrderConfirmed, "order-service", order.ID, payload)
	case model.OrderStatusShipped, model.OrderStatusOutForDelivery:
		topic = OrderShippedTopic
		payload := events.OrderShippedEvent{
			OrderID:   order.ID,
			UserID:    order.UserID,
			ShippedAt: now,
		}
		envelope, _ = events.NewEventEnvelopeWithAggregate(events.EventTypeOrderShipped, "order-service", order.ID, payload)
	case model.OrderStatusDelivered:
		topic = OrderDeliveredTopic
		payload := events.OrderDeliveredEvent{
			OrderID:     order.ID,
			UserID:      order.UserID,
			DeliveredAt: now,
		}
		envelope, _ = events.NewEventEnvelopeWithAggregate(events.EventTypeOrderDelivered, "order-service", order.ID, payload)
	case model.OrderStatusCancelled:
		topic = OrderCancelledTopic
		payload := events.OrderCancelledEvent{
			OrderID:     order.ID,
			UserID:      order.UserID,
			Reason:      reasonStr,
			CancelledAt: now,
		}
		envelope, _ = events.NewEventEnvelopeWithAggregate(events.EventTypeOrderCancelled, "order-service", order.ID, payload)
	case model.OrderStatusPaymentFailed:
		topic = OrderPaymentFailedTopic
		payload := events.OrderPaymentFailedEvent{
			OrderID:  order.ID,
			UserID:   order.UserID,
			Reason:   reasonStr,
			FailedAt: now,
		}
		envelope, _ = events.NewEventEnvelopeWithAggregate(events.EventTypeOrderPaymentFailed, "order-service", order.ID, payload)
	}

	updatedOrder, err := s.repo.UpdateOrderStatusWithOutbox(ctx, order.ID, orderStatus, envelope, topic, reason...)
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
	paymentConfirmedEvt := events.OrderPaymentConfirmedEvent{
		OrderID:       order.ID,
		OrderNumber:   order.OrderNumber,
		UserID:        order.UserID,
		StoreID:       order.StoreID,
		TotalAmount:   order.TotalAmount,
		Currency:      order.Currency,
		PaymentStatus: string(model.PaymentStatusPaid),
		PaidAt:        now,
	}

	envelope, err := events.NewEventEnvelopeWithAggregate(
		events.EventTypeOrderPaymentConfirmed,
		"order-service",
		order.ID,
		paymentConfirmedEvt,
	)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to create OrderPaymentConfirmed event envelope")
	}
	if correlationID != "" {
		envelope.CorrelationID = correlationID
	}
	if traceID != "" {
		envelope.TraceID = traceID
	}

	updatedOrder, err := s.repo.UpdateOrderStatusWithOutbox(
		ctx,
		order.ID,
		model.OrderStatusPaid,
		envelope,
		events.TopicOrderPaymentConfirmed,
		"Payment successful",
	)
	if err != nil {
		return nil, err
	}

	s.log.Info("Order transitioned to PAID and OrderPaymentConfirmed event recorded in outbox",
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
		UserID:    order.UserID,
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
		order.ID,
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
	// Only user who ordered the product can cancel; not even admin has right to cancel.
	callerUserID := strings.TrimSpace(userID)
	callerRole := ""
	if userCtx, ok := auth.UserFromContext(ctx); ok && userCtx != nil {
		if callerUserID == "" {
			callerUserID = userCtx.UserID
		}
		callerRole = strings.ToUpper(strings.TrimSpace(userCtx.Role))
	}
	if callerRole == "" {
		callerRole = strings.ToUpper(strings.TrimSpace(grpcclient.GetUserRole(ctx)))
	}

	if callerRole == "ADMIN" || callerRole == auth.RoleAdmin {
		return nil, appErrors.Forbidden("admins do not have permission to cancel orders; only the customer who placed the order or the merchant can cancel")
	}

	if callerRole == "MERCHANT" || callerRole == auth.RoleMerchant {
		// Merchant cancellation rules:
		// 1. Once order transitions to CONFIRMED (or beyond), merchant has NO permission to cancel/reject.
		if order.Status == model.OrderStatusConfirmed ||
			order.Status == model.OrderStatusProcessing ||
			order.Status == model.OrderStatusShipped ||
			order.Status == model.OrderStatusOutForDelivery ||
			order.Status == model.OrderStatusDelivered {
			return nil, appErrors.FailedPrecondition("cannot cancel or reject order: confirmed orders cannot be cancelled by the merchant")
		}
		// 2. Before confirmation: Merchant is allowed to cancel paid orders only.
		if !model.CanMerchantCancelOrder(order.Status, order.PaymentStatus) {
			return nil, appErrors.FailedPrecondition("cannot cancel order: merchants are allowed to cancel paid orders only before confirming them")
		}
	} else {
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
		UserID:      order.UserID,
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
