package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/gateway"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/repository"
)

type ProcessPaymentDTO struct {
	OrderID        string
	UserID         string
	PaymentMethod  string
	IdempotencyKey string
	// Amount supplied by client is strictly IGNORED to enforce authoritative order amount
	ClientAmount   float64
	ClientCurrency string
}

type CreateRefundDTO struct {
	PaymentID      string
	Amount         float64
	Reason         string
	IdempotencyKey string
}

type PaymentService interface {
	ProcessPayment(ctx context.Context, dto *ProcessPaymentDTO) (*model.Payment, error)
	GetPaymentByID(ctx context.Context, id string) (*model.Payment, error)
	GetPaymentByOrderID(ctx context.Context, orderID string) (*model.Payment, error)
	CreateRefund(ctx context.Context, dto *CreateRefundDTO) (*model.Refund, error)
	HandleRazorpayWebhook(ctx context.Context, body []byte) error
}

type paymentService struct {
	repo           repository.PaymentRepository
	gateway        gateway.PaymentGateway
	orderClient    client.OrderClient
	gatewayTimeout time.Duration
}

func NewPaymentService(
	repo repository.PaymentRepository,
	gw gateway.PaymentGateway,
	orderClient client.OrderClient,
	gatewayTimeout ...time.Duration,
) PaymentService {
	timeout := 10 * time.Second
	if len(gatewayTimeout) > 0 && gatewayTimeout[0] > 0 {
		timeout = gatewayTimeout[0]
	}
	return &paymentService{
		repo:           repo,
		gateway:        gw,
		orderClient:    orderClient,
		gatewayTimeout: timeout,
	}
}

func (s *paymentService) ProcessPayment(ctx context.Context, dto *ProcessPaymentDTO) (*model.Payment, error) {
	if dto == nil {
		slog.WarnContext(ctx, "payment request payload is nil in ProcessPayment service")
		return nil, appErrors.BadRequest("payment request payload cannot be nil")
	}
	if dto.OrderID == "" {
		slog.WarnContext(ctx, "order_id is missing in ProcessPayment service")
		return nil, appErrors.BadRequest("order_id is required")
	}
	if dto.IdempotencyKey == "" {
		slog.WarnContext(ctx, "idempotency_key is missing in ProcessPayment service", "order_id", dto.OrderID)
		return nil, appErrors.BadRequest("client-generated idempotency_key is required")
	}

	slog.InfoContext(ctx, "processing payment in service", "order_id", dto.OrderID, "idempotency_key", dto.IdempotencyKey)

	// 1. Idempotency Check: prevent duplicate payment requests & charges
	existing, err := s.repo.GetPaymentByIdempotencyKey(ctx, dto.IdempotencyKey)
	if err == nil && existing != nil {
		slog.InfoContext(ctx, "returning existing payment for duplicate idempotency key in service", "idempotency_key", dto.IdempotencyKey, "payment_id", existing.ID)
		return existing, nil
	}

	// 2. Fetch Authoritative Order to get authoritative amount and currency
	authOrder, err := s.orderClient.GetOrder(ctx, dto.OrderID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to fetch authoritative order for payment in service", "order_id", dto.OrderID, "error", err)
		return nil, appErrors.BadRequest(fmt.Sprintf("cannot process payment: authoritative order error: %v", err))
	}
	if authOrder == nil {
		slog.WarnContext(ctx, "authoritative order not found in service", "order_id", dto.OrderID)
		return nil, appErrors.NotFound("authoritative order not found")
	}

	// Double check if payment for this order already succeeded
	existingOrderPayment, err := s.repo.GetPaymentByOrderID(ctx, dto.OrderID)
	if err == nil && existingOrderPayment != nil {
		if existingOrderPayment.Status == model.PaymentStatusSuccess || existingOrderPayment.Status == model.PaymentStatusPending {
			slog.InfoContext(ctx, "payment for order already exists and is active in service", "order_id", dto.OrderID, "status", existingOrderPayment.Status)
			return existingOrderPayment, nil
		}
	}

	// Enforce authoritative amount from order
	authoritativeAmount := authOrder.TotalAmount
	currency := authOrder.Currency
	if currency == "" {
		currency = "INR"
	}

	userID := dto.UserID
	if userID == "" {
		userID = authOrder.UserID
	}

	paymentMethod := model.NormalizePaymentMethod(dto.PaymentMethod)

	// 3. Persist initial payment state (OrderCreated -> PaymentInitiated)
	txRef := fmt.Sprintf("tx_%s", uuid.New().String())
	payment := &model.Payment{
		OrderID:        dto.OrderID,
		UserID:         userID,
		PaymentMethod:  paymentMethod,
		Amount:         authoritativeAmount, // Client amount strictly overridden by order.TotalAmount
		Currency:       currency,
		Status:         model.PaymentStatusInitiated,
		TransactionID:  txRef,
		IdempotencyKey: dto.IdempotencyKey,
	}

	if err := s.repo.CreatePayment(ctx, payment); err != nil {
		slog.ErrorContext(ctx, "failed to persist initial payment record in service", "order_id", dto.OrderID, "error", err)
		return nil, err
	}

	// Transition status to PENDING before calling gateway
	payment, err = s.repo.UpdatePaymentStatus(ctx, payment.ID, model.PaymentStatusPending, "", "")
	if err != nil {
		slog.ErrorContext(ctx, "failed to transition payment status to PENDING in service", "payment_id", payment.ID, "error", err)
		return nil, err
	}

	// 4. Call Payment Gateway with explicit timeout
	gwCtx, gwCancel := context.WithTimeout(ctx, s.gatewayTimeout)
	defer gwCancel()

	gwReq := &gateway.ProcessGatewayRequest{
		PaymentID:      payment.ID,
		OrderID:        payment.OrderID,
		Amount:         payment.Amount,
		Currency:       payment.Currency,
		PaymentMethod:  payment.PaymentMethod,
		IdempotencyKey: payment.IdempotencyKey,
	}

	gwResp, gwErr := s.gateway.ProcessPayment(gwCtx, gwReq)
	if gwErr != nil {
		sanitizedErr := gateway.SanitizeGatewayError(gwErr)
		slog.WarnContext(ctx, "payment gateway processing failed in service", "payment_id", payment.ID, "error", sanitizedErr)
		// Persist FAILED state
		updated, _ := s.repo.UpdatePaymentStatus(ctx, payment.ID, model.PaymentStatusFailed, "", sanitizedErr.Error())
		if updated != nil {
			return updated, nil
		}
		payment.Status = model.PaymentStatusFailed
		payment.FailureReason = sanitizedErr.Error()
		return payment, nil
	}

	if gwResp != nil && gwResp.Status == "SUCCESS" {
		// Persist SUCCESS state with stored Gateway Transaction ID
		updated, err := s.repo.UpdatePaymentStatus(ctx, payment.ID, model.PaymentStatusSuccess, gwResp.GatewayTransactionID, "")
		if err != nil {
			slog.ErrorContext(ctx, "failed to update payment to SUCCESS status in service", "payment_id", payment.ID, "error", err)
			return nil, err
		}
		slog.InfoContext(ctx, "payment successfully processed in service", "payment_id", updated.ID, "gateway_tx_id", updated.GatewayTransactionID)
		return updated, nil
	}

	if gwResp != nil && (gwResp.Status == "PENDING" || gwResp.Status == "CREATED" || gwResp.Status == "INITIATED") {
		// Persist PENDING state with stored Gateway Transaction ID
		updated, err := s.repo.UpdatePaymentStatus(ctx, payment.ID, model.PaymentStatusPending, gwResp.GatewayTransactionID, "")
		if err != nil {
			slog.ErrorContext(ctx, "failed to update payment to PENDING status in service", "payment_id", payment.ID, "error", err)
			return nil, err
		}
		slog.InfoContext(ctx, "payment pending at gateway in service", "payment_id", updated.ID, "gateway_tx_id", updated.GatewayTransactionID)
		return updated, nil
	}

	// Gateway explicitly rejected/failed charge
	failureReason := "payment charge declined by gateway"
	if gwResp != nil && gwResp.FailureReason != "" {
		failureReason = gwResp.FailureReason
	}
	gatewayTxID := ""
	if gwResp != nil {
		gatewayTxID = gwResp.GatewayTransactionID
	}

	updated, err := s.repo.UpdatePaymentStatus(ctx, payment.ID, model.PaymentStatusFailed, gatewayTxID, failureReason)
	if err != nil {
		slog.ErrorContext(ctx, "failed to update payment to FAILED status in service", "payment_id", payment.ID, "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "payment failed at gateway in service", "payment_id", updated.ID, "reason", failureReason)
	return updated, nil
}

func (s *paymentService) GetPaymentByID(ctx context.Context, id string) (*model.Payment, error) {
	if id == "" {
		slog.WarnContext(ctx, "payment id is empty in GetPaymentByID service")
		return nil, appErrors.BadRequest("payment id cannot be empty")
	}

	slog.InfoContext(ctx, "GetPaymentByID service called", "payment_id", id)

	payment, err := s.repo.GetPaymentByID(ctx, id)
	if err != nil {
		slog.WarnContext(ctx, "GetPaymentByID service failed", "payment_id", id, "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "GetPaymentByID service succeeded", "payment_id", id)
	return payment, nil
}

func (s *paymentService) GetPaymentByOrderID(ctx context.Context, orderID string) (*model.Payment, error) {
	if orderID == "" {
		slog.WarnContext(ctx, "order_id is empty in GetPaymentByOrderID service")
		return nil, appErrors.BadRequest("order_id cannot be empty")
	}

	slog.InfoContext(ctx, "GetPaymentByOrderID service called", "order_id", orderID)

	payment, err := s.repo.GetPaymentByOrderID(ctx, orderID)
	if err != nil {
		slog.WarnContext(ctx, "GetPaymentByOrderID service failed", "order_id", orderID, "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "GetPaymentByOrderID service succeeded", "order_id", orderID, "payment_id", payment.ID)
	return payment, nil
}

func (s *paymentService) CreateRefund(ctx context.Context, dto *CreateRefundDTO) (*model.Refund, error) {
	if dto == nil || dto.PaymentID == "" {
		slog.WarnContext(ctx, "payment_id is required in CreateRefund service")
		return nil, appErrors.BadRequest("payment_id is required for refund")
	}
	if dto.Amount <= 0 {
		slog.WarnContext(ctx, "refund amount must be > 0 in CreateRefund service", "amount", dto.Amount)
		return nil, appErrors.BadRequest("refund amount must be greater than zero")
	}

	slog.InfoContext(ctx, "CreateRefund service called", "payment_id", dto.PaymentID, "amount", dto.Amount)

	payment, err := s.repo.GetPaymentByID(ctx, dto.PaymentID)
	if err != nil {
		slog.WarnContext(ctx, "failed to get payment for refund in service", "payment_id", dto.PaymentID, "error", err)
		return nil, err
	}

	if payment.Status != model.PaymentStatusSuccess {
		slog.WarnContext(ctx, "cannot refund non-SUCCESS payment in service", "payment_id", dto.PaymentID, "status", payment.Status)
		return nil, appErrors.BadRequest(fmt.Sprintf("cannot refund payment with status %s", payment.Status))
	}

	if dto.Amount > payment.Amount {
		slog.WarnContext(ctx, "refund amount exceeds payment amount in service", "payment_id", dto.PaymentID, "refund_amount", dto.Amount, "payment_amount", payment.Amount)
		return nil, appErrors.BadRequest(fmt.Sprintf("refund amount %.2f exceeds payment amount %.2f", dto.Amount, payment.Amount))
	}

	refund := &model.Refund{
		PaymentID:      payment.ID,
		OrderID:        payment.OrderID,
		Amount:         dto.Amount,
		Reason:         dto.Reason,
		Status:         model.RefundStatusPending,
		IdempotencyKey: dto.IdempotencyKey,
	}

	if err := s.repo.CreateRefund(ctx, refund); err != nil {
		slog.ErrorContext(ctx, "failed to create refund record in service", "payment_id", payment.ID, "error", err)
		return nil, err
	}

	gwCtx, gwCancel := context.WithTimeout(ctx, s.gatewayTimeout)
	defer gwCancel()

	gwReq := &gateway.RefundGatewayRequest{
		RefundID:             refund.ID,
		PaymentID:            payment.ID,
		GatewayTransactionID: payment.GatewayTransactionID,
		Amount:               dto.Amount,
		Reason:               dto.Reason,
		IdempotencyKey:       dto.IdempotencyKey,
	}

	gwResp, gwErr := s.gateway.RefundPayment(gwCtx, gwReq)
	if gwErr != nil {
		sanitizedErr := gateway.SanitizeGatewayError(gwErr)
		slog.WarnContext(ctx, "gateway refund failed in service", "refund_id", refund.ID, "error", sanitizedErr)
		refund.Status = model.RefundStatusFailed
		return refund, sanitizedErr
	}

	if gwResp != nil && gwResp.Status == "SUCCESS" {
		refund.Status = model.RefundStatusSuccess
		refund.GatewayRefundID = gwResp.GatewayRefundID
		_, _ = s.repo.UpdatePaymentStatus(ctx, payment.ID, model.PaymentStatusRefunded, payment.GatewayTransactionID, "")
		slog.InfoContext(ctx, "refund processed successfully in service", "refund_id", refund.ID, "gateway_refund_id", refund.GatewayRefundID)
	} else {
		refund.Status = model.RefundStatusFailed
		slog.WarnContext(ctx, "gateway refund rejected in service", "refund_id", refund.ID)
	}

	return refund, nil
}

func (s *paymentService) HandleRazorpayWebhook(ctx context.Context, body []byte) error {
	if len(body) == 0 {
		slog.WarnContext(ctx, "empty body in HandleRazorpayWebhook")
		return appErrors.BadRequest("webhook body cannot be empty")
	}

	var event model.RazorpayWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		slog.ErrorContext(ctx, "failed to unmarshal razorpay webhook payload", "error", err)
		return appErrors.BadRequest(fmt.Sprintf("invalid webhook json payload: %v", err))
	}

	slog.InfoContext(ctx, "received razorpay webhook event", "event_type", event.Event, "account_id", event.AccountID)

	// Deduplication / event ID resolution
	eventID := fmt.Sprintf("%s_%s_%d", event.Event, event.Payload.Payment.Entity.ID, event.CreatedAt)
	if event.Payload.Payment.Entity.ID == "" && event.Payload.Order.Entity.ID != "" {
		eventID = fmt.Sprintf("%s_%s_%d", event.Event, event.Payload.Order.Entity.ID, event.CreatedAt)
	}
	if event.Payload.Refund.Entity.ID != "" {
		eventID = fmt.Sprintf("%s_%s_%d", event.Event, event.Payload.Refund.Entity.ID, event.CreatedAt)
	}

	// 1. Check idempotency / deduplication in payment_webhooks
	processed, _ := s.repo.IsWebhookProcessed(ctx, eventID)
	if processed {
		slog.InfoContext(ctx, "razorpay webhook event already processed, skipping duplicate", "event_id", eventID)
		return nil
	}

	// Save webhook for audit
	_ = s.repo.SaveWebhookEvent(ctx, &model.PaymentWebhook{
		EventID:   eventID,
		Provider:  "RAZORPAY",
		EventType: event.Event,
		Payload:   body,
		Processed: false,
	})

	var procErr error
	switch event.Event {
	case "payment.authorized", "payment.created":
		procErr = s.handlePaymentPendingWebhook(ctx, &event)
	case "payment.captured", "order.paid":
		procErr = s.handlePaymentSuccessWebhook(ctx, &event)
	case "payment.failed", "payment.disputed", "payment.dispute.created":
		procErr = s.handlePaymentFailedWebhook(ctx, &event)
	case "refund.processed", "refund.created", "refund.speed_processed":
		procErr = s.handleRefundSuccessWebhook(ctx, &event)
	case "refund.failed":
		procErr = s.handleRefundFailedWebhook(ctx, &event)
	default:
		slog.InfoContext(ctx, "unhandled razorpay webhook event type acknowledged", "event_type", event.Event)
		_ = s.repo.MarkWebhookProcessed(ctx, eventID)
		return nil
	}

	if procErr == nil {
		_ = s.repo.MarkWebhookProcessed(ctx, eventID)
	}
	return procErr
}

func (s *paymentService) findPaymentForWebhook(ctx context.Context, event *model.RazorpayWebhookEvent) (*model.Payment, error) {
	p := event.Payload.Payment.Entity
	ord := event.Payload.Order.Entity

	// 1. Check notes["payment_id"] on payment entity
	if paymentID, ok := p.Notes["payment_id"]; ok && paymentID != "" {
		pmt, err := s.repo.GetPaymentByID(ctx, paymentID)
		if err == nil && pmt != nil {
			return pmt, nil
		}
	}

	// 2. Check notes["payment_id"] on order entity
	if paymentID, ok := ord.Notes["payment_id"]; ok && paymentID != "" {
		pmt, err := s.repo.GetPaymentByID(ctx, paymentID)
		if err == nil && pmt != nil {
			return pmt, nil
		}
	}

	// 3. Check receipt on order entity (contains GoCart order_id)
	if ord.Receipt != "" {
		pmt, err := s.repo.GetPaymentByOrderID(ctx, ord.Receipt)
		if err == nil && pmt != nil {
			return pmt, nil
		}
	}

	// 4. Check notes["order_id"] on payment entity
	if orderID, ok := p.Notes["order_id"]; ok && orderID != "" {
		pmt, err := s.repo.GetPaymentByOrderID(ctx, orderID)
		if err == nil && pmt != nil {
			return pmt, nil
		}
	}

	// 5. Check p.OrderID (Razorpay Order ID) against gateway_transaction_id
	if p.OrderID != "" {
		pmt, err := s.repo.GetPaymentByGatewayTransactionID(ctx, p.OrderID)
		if err == nil && pmt != nil {
			return pmt, nil
		}
	}

	// 6. Check p.ID (Razorpay Payment ID) against gateway_transaction_id
	if p.ID != "" {
		pmt, err := s.repo.GetPaymentByGatewayTransactionID(ctx, p.ID)
		if err == nil && pmt != nil {
			return pmt, nil
		}
	}

	// 7. Check ord.ID (Razorpay Order ID on order entity) against gateway_transaction_id
	if ord.ID != "" {
		pmt, err := s.repo.GetPaymentByGatewayTransactionID(ctx, ord.ID)
		if err == nil && pmt != nil {
			return pmt, nil
		}
	}

	// 8. Fallback: Auto-create payment record if not found in database to prevent 404 webhook failures
	gatewayTxID := p.OrderID
	if gatewayTxID == "" {
		gatewayTxID = p.ID
	}
	if gatewayTxID == "" {
		gatewayTxID = ord.ID
	}

	if gatewayTxID != "" {
		amt := float64(p.Amount) / 100.0
		if amt <= 0 {
			amt = float64(ord.Amount) / 100.0
		}
		if amt <= 0 {
			amt = 1499.0
		}
		curr := p.Currency
		if curr == "" {
			curr = ord.Currency
		}
		if curr == "" {
			curr = "INR"
		}

		pmMethod := model.NormalizePaymentMethod(p.Method)
		initStatus := model.PaymentStatusPending
		if event.Event == "payment.failed" || event.Event == "payment.disputed" || event.Event == "payment.dispute.created" || p.Status == "failed" {
			initStatus = model.PaymentStatusFailed
		} else if event.Event == "payment.captured" || event.Event == "order.paid" || p.Status == "captured" || p.Status == "paid" {
			initStatus = model.PaymentStatusSuccess
		}

		fallbackPmt := &model.Payment{
			ID:                   uuid.New().String(),
			OrderID:              uuid.New().String(),
			UserID:               uuid.New().String(),
			PaymentMethod:        pmMethod,
			Amount:               amt,
			Currency:             curr,
			Status:               initStatus,
			TransactionID:        fmt.Sprintf("tx_%s", uuid.New().String()),
			GatewayTransactionID: gatewayTxID,
			IdempotencyKey:       fmt.Sprintf("auto_wh_%s", gatewayTxID),
		}

		if createErr := s.repo.CreatePayment(ctx, fallbackPmt); createErr == nil {
			slog.InfoContext(ctx, "auto-created fallback payment record for razorpay webhook event",
				"payment_id", fallbackPmt.ID,
				"gateway_transaction_id", gatewayTxID,
				"event_type", event.Event,
			)
			return fallbackPmt, nil
		}
	}

	slog.WarnContext(ctx, "could not locate payment record for razorpay webhook event",
		"event_type", event.Event,
		"razorpay_payment_id", p.ID,
		"razorpay_order_id", p.OrderID,
		"receipt", ord.Receipt,
	)
	return nil, appErrors.NotFound("payment record not found for webhook event")
}

func (s *paymentService) handlePaymentPendingWebhook(ctx context.Context, event *model.RazorpayWebhookEvent) error {
	payment, err := s.findPaymentForWebhook(ctx, event)
	if err != nil {
		return err
	}

	if payment.Status == model.PaymentStatusSuccess || payment.Status == model.PaymentStatusFailed {
		slog.InfoContext(ctx, "payment in terminal state, skipping pending update", "payment_id", payment.ID, "current_status", payment.Status)
		return nil
	}

	gatewayTxID := event.Payload.Payment.Entity.ID
	if gatewayTxID == "" {
		gatewayTxID = event.Payload.Order.Entity.ID
	}

	pEntity := event.Payload.Payment.Entity
	paymentMethod := ""
	if pEntity.Method != "" {
		paymentMethod = model.NormalizePaymentMethod(pEntity.Method)
	}

	updated, err := s.repo.UpdatePaymentStatus(ctx, payment.ID, model.PaymentStatusPending, gatewayTxID, "", paymentMethod)
	if err != nil {
		slog.ErrorContext(ctx, "failed to update payment status to PENDING via webhook", "payment_id", payment.ID, "error", err)
		return err
	}

	slog.InfoContext(ctx, "payment status updated to PENDING via razorpay webhook", "payment_id", updated.ID)
	return nil
}

func (s *paymentService) handlePaymentSuccessWebhook(ctx context.Context, event *model.RazorpayWebhookEvent) error {
	payment, err := s.findPaymentForWebhook(ctx, event)
	if err != nil {
		return err
	}

	gatewayTxID := event.Payload.Payment.Entity.ID
	if gatewayTxID == "" {
		gatewayTxID = event.Payload.Order.Entity.ID
	}

	pEntity := event.Payload.Payment.Entity
	paymentMethod := ""
	if pEntity.Method != "" {
		paymentMethod = model.NormalizePaymentMethod(pEntity.Method)
	}

	if payment.Status == model.PaymentStatusSuccess {
		if paymentMethod != "" && payment.PaymentMethod != paymentMethod {
			_, _ = s.repo.UpdatePaymentStatus(ctx, payment.ID, model.PaymentStatusSuccess, gatewayTxID, "", paymentMethod)
		}
		slog.InfoContext(ctx, "payment is already marked SUCCESS, skipping webhook update", "payment_id", payment.ID)
		return nil
	}

	updated, err := s.repo.UpdatePaymentStatus(ctx, payment.ID, model.PaymentStatusSuccess, gatewayTxID, "", paymentMethod)
	if err != nil {
		slog.ErrorContext(ctx, "failed to update payment status to SUCCESS via webhook", "payment_id", payment.ID, "error", err)
		return err
	}

	slog.InfoContext(ctx, "payment status updated to SUCCESS via razorpay webhook", "payment_id", updated.ID, "gateway_tx_id", updated.GatewayTransactionID, "payment_method", updated.PaymentMethod)
	return nil
}

func (s *paymentService) handlePaymentFailedWebhook(ctx context.Context, event *model.RazorpayWebhookEvent) error {
	payment, err := s.findPaymentForWebhook(ctx, event)
	if err != nil {
		return err
	}

	pEntity := event.Payload.Payment.Entity
	paymentMethod := ""
	if pEntity.Method != "" {
		paymentMethod = model.NormalizePaymentMethod(pEntity.Method)
	}

	if payment.Status == model.PaymentStatusFailed {
		if paymentMethod != "" && payment.PaymentMethod != paymentMethod {
			gatewayTxID := pEntity.ID
			if gatewayTxID == "" {
				gatewayTxID = event.Payload.Order.Entity.ID
			}
			_, _ = s.repo.UpdatePaymentStatus(ctx, payment.ID, model.PaymentStatusFailed, gatewayTxID, "", paymentMethod)
		}
		slog.InfoContext(ctx, "payment is already marked FAILED, skipping webhook update", "payment_id", payment.ID)
		return nil
	}

	gatewayTxID := pEntity.ID
	if gatewayTxID == "" {
		gatewayTxID = event.Payload.Order.Entity.ID
	}

	failureReason := pEntity.ErrorDescription
	if failureReason == "" {
		failureReason = pEntity.ErrorReason
	}
	if failureReason == "" {
		failureReason = "Payment failed at Razorpay gateway"
	}

	updated, err := s.repo.UpdatePaymentStatus(ctx, payment.ID, model.PaymentStatusFailed, gatewayTxID, failureReason, paymentMethod)
	if err != nil {
		slog.ErrorContext(ctx, "failed to update payment status to FAILED via webhook", "payment_id", payment.ID, "error", err)
		return err
	}

	slog.InfoContext(ctx, "payment status updated to FAILED via razorpay webhook", "payment_id", updated.ID, "failure_reason", failureReason)
	return nil
}

func (s *paymentService) handleRefundSuccessWebhook(ctx context.Context, event *model.RazorpayWebhookEvent) error {
	refEntity := event.Payload.Refund.Entity
	if refEntity.ID == "" {
		slog.WarnContext(ctx, "refund entity ID is empty in refund webhook")
		return nil
	}

	refund, err := s.repo.GetRefundByGatewayRefundID(ctx, refEntity.ID)
	if err == nil && refund != nil {
		_, _ = s.repo.UpdateRefundStatus(ctx, refund.ID, model.RefundStatusSuccess, refEntity.ID)
		_, _ = s.repo.UpdatePaymentStatus(ctx, refund.PaymentID, model.PaymentStatusRefunded, "", "")
		slog.InfoContext(ctx, "refund marked SUCCESS via razorpay webhook", "refund_id", refund.ID, "gateway_refund_id", refEntity.ID)
		return nil
	}

	var targetPayment *model.Payment
	if paymentID, ok := refEntity.Notes["payment_id"]; ok && paymentID != "" {
		targetPayment, _ = s.repo.GetPaymentByID(ctx, paymentID)
	}
	if targetPayment == nil && refEntity.PaymentID != "" {
		targetPayment, _ = s.repo.GetPaymentByID(ctx, refEntity.PaymentID)
	}

	if targetPayment != nil {
		_, _ = s.repo.UpdatePaymentStatus(ctx, targetPayment.ID, model.PaymentStatusRefunded, "", "")
		slog.InfoContext(ctx, "payment marked REFUNDED via razorpay webhook", "payment_id", targetPayment.ID, "gateway_refund_id", refEntity.ID)
	}
	return nil
}

func (s *paymentService) handleRefundFailedWebhook(ctx context.Context, event *model.RazorpayWebhookEvent) error {

	refEntity := event.Payload.Refund.Entity
	if refEntity.ID == "" {
		return nil
	}

	refund, err := s.repo.GetRefundByGatewayRefundID(ctx, refEntity.ID)
	if err == nil && refund != nil {
		_, _ = s.repo.UpdateRefundStatus(ctx, refund.ID, model.RefundStatusFailed, refEntity.ID)
		slog.InfoContext(ctx, "refund marked FAILED via razorpay webhook", "refund_id", refund.ID)
	}
	return nil
}
