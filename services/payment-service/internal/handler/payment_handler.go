package handler

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/gateway"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/service"
)

type PaymentHandler struct {
	paymentRepo    repository.PaymentRepository
	paymentGW      gateway.PaymentGateway
	paymentService service.PaymentService
	gwConfig       gateway.GatewayConfig
	log            *slog.Logger
}

func NewPaymentHandler(paymentRepo repository.PaymentRepository, paymentGW gateway.PaymentGateway, gwConfig gateway.GatewayConfig, log *slog.Logger, paymentService ...service.PaymentService) *PaymentHandler {
	if log == nil {
		log = slog.Default()
	}
	var svc service.PaymentService
	if len(paymentService) > 0 {
		svc = paymentService[0]
	}
	return &PaymentHandler{
		paymentRepo:    paymentRepo,
		paymentGW:      paymentGW,
		paymentService: svc,
		gwConfig:       gwConfig,
		log:            log,
	}
}

func (h *PaymentHandler) RegisterRoutes(app *fiber.App) {
	app.Get("/get-key", h.GetKey)
	app.Post("/create-order", h.CreateOrder)
	app.All("/payment-callback", h.PaymentCallback)
	app.Get("/", h.ServeTestUI)
	app.Get("/test-ui", h.ServeTestUI)
	app.Post("/api/process-refund", h.ProcessRefund)
	app.Get("/api/order-payment/:order_id", h.GetOrderPayment)
}

func (h *PaymentHandler) GetKey(c *fiber.Ctx) error {
	key := h.gwConfig.RazorpayKeyID
	if key == "" {
		key = "rzp_test_xxxx"
	}
	return c.JSON(fiber.Map{"key": key})
}

func (h *PaymentHandler) CreateOrder(c *fiber.Ctx) error {
	type orderReq struct {
		OrderID          string  `json:"order_id"`
		UserID           string  `json:"user_id"`
		Amount           float64 `json:"amount"`
		Currency         string  `json:"currency"`
		PaymentMethod    string  `json:"payment_method"`
		PaymentMethodAlt string  `json:"paymentMethod"`
	}
	var req orderReq
	_ = c.BodyParser(&req)
	if req.Amount <= 0 {
		req.Amount = 1499
	}
	if req.Currency == "" {
		req.Currency = "INR"
	}
	method := req.PaymentMethod
	if method == "" {
		method = req.PaymentMethodAlt
	}
	if method == "" {
		method = "NET_BANKING"
	}
	paymentMethod := model.NormalizePaymentMethod(method)

	orderID := req.OrderID
	if orderID != "" && h.paymentRepo != nil {
		existing, err := h.paymentRepo.GetPaymentByOrderID(c.UserContext(), orderID)
		if err == nil && existing != nil && existing.Status == model.PaymentStatusPending && existing.GatewayTransactionID != "" {
			return c.JSON(fiber.Map{
				"id":         existing.GatewayTransactionID,
				"payment_id": existing.ID,
				"amount":     int64(existing.Amount * 100),
				"currency":   existing.Currency,
				"status":     "created",
				"is_real":    true,
			})
		}
	}
	if orderID == "" {
		orderID = uuid.New().String()
	}

	userID := req.UserID
	if userID == "" {
		userID = uuid.New().String()
	}

	// If real Razorpay key & secret are provided, create genuine Razorpay Order ID via API
	if h.gwConfig.RazorpayKeyID != "" && h.gwConfig.RazorpayKeySecret != "" && !strings.Contains(h.gwConfig.RazorpayKeyID, "xxxx") {
		paymentID := uuid.New().String()
		idempKey := fmt.Sprintf("idemp_%d", time.Now().UnixNano())

		rzpResp, err := h.paymentGW.ProcessPayment(c.UserContext(), &gateway.ProcessGatewayRequest{
			PaymentID:      paymentID,
			OrderID:        orderID,
			Amount:         req.Amount,
			Currency:       req.Currency,
			PaymentMethod:  paymentMethod,
			IdempotencyKey: idempKey,
		})

		if err == nil && rzpResp != nil && rzpResp.GatewayTransactionID != "" {
			pmt := &model.Payment{
				ID:                   paymentID,
				OrderID:              orderID,
				UserID:               userID,
				PaymentMethod:        paymentMethod,
				Amount:               req.Amount,
				Currency:             req.Currency,
				Status:               model.PaymentStatusPending,
				TransactionID:        fmt.Sprintf("tx_%s", uuid.New().String()),
				GatewayTransactionID: rzpResp.GatewayTransactionID,
				IdempotencyKey:       idempKey,
			}
			if h.paymentRepo != nil {
				if createErr := h.paymentRepo.CreatePayment(c.UserContext(), pmt); createErr != nil {
					h.log.Error("failed to persist payment record for razorpay order", "error", createErr)
				} else {
					h.log.Info("persisted pending razorpay payment in database", "payment_id", pmt.ID, "razorpay_order_id", rzpResp.GatewayTransactionID)
				}
			}

			return c.JSON(fiber.Map{
				"id":         rzpResp.GatewayTransactionID,
				"payment_id": paymentID,
				"amount":     int64(req.Amount * 100),
				"currency":   req.Currency,
				"status":     "created",
				"is_real":    true,
			})
		}
	}

	// If test / placeholder mode, leave order ID empty so Razorpay SDK allows standard client-side checkout
	return c.JSON(fiber.Map{
		"id":       "",
		"amount":   int64(req.Amount * 100),
		"currency": req.Currency,
		"status":   "created",
		"is_real":  false,
	})
}

func (h *PaymentHandler) PaymentCallback(c *fiber.Ctx) error {
	type cbJSON struct {
		OrderID           string  `json:"order_id"`
		Amount            float64 `json:"amount"`
		RazorpayOrderID   string  `json:"razorpay_order_id"`
		RazorpayPaymentID string  `json:"razorpay_payment_id"`
		RazorpaySignature string  `json:"razorpay_signature"`
		Status            string  `json:"status"`
		ErrorCode         string  `json:"error_code"`
		ErrorDescription  string  `json:"error_description"`
	}
	var cb cbJSON
	_ = c.BodyParser(&cb)

	orderID := c.FormValue("order_id", c.Query("order_id", c.Query("orderId", "")))
	if orderID == "" {
		orderID = cb.OrderID
	}
	if orderID == "" {
		orderID = c.FormValue("razorpay_order_id", c.Query("razorpay_order_id", ""))
		if orderID == "" {
			orderID = cb.RazorpayOrderID
		}
	}
	paymentID := c.FormValue("razorpay_payment_id", c.Query("razorpay_payment_id", c.Query("paymentId", "")))
	if paymentID == "" {
		paymentID = cb.RazorpayPaymentID
	}
	signature := c.FormValue("razorpay_signature", c.Query("razorpay_signature", c.Query("signature", "")))
	if signature == "" {
		signature = cb.RazorpaySignature
	}
	statusParam := strings.ToLower(c.FormValue("status", c.Query("status", "")))
	if statusParam == "" {
		statusParam = strings.ToLower(cb.Status)
	}
	errorCode := c.FormValue("error_code", c.Query("error_code", c.Query("error[code]", "")))
	if errorCode == "" {
		errorCode = cb.ErrorCode
	}
	errorDesc := c.FormValue("error_description", c.Query("error_description", c.Query("error[description]", c.Query("error_reason", c.Query("error", "")))))
	if errorDesc == "" {
		errorDesc = cb.ErrorDescription
	}

	isFailed := statusParam == "failed" || statusParam == "failure" || statusParam == "error" || errorCode != ""

	if (orderID != "" || paymentID != "") && h.paymentRepo != nil {
		lookupID := orderID
		if lookupID == "" {
			lookupID = paymentID
		}
		pmt, err := h.paymentRepo.GetPaymentByID(c.UserContext(), lookupID)
		if (err != nil || pmt == nil) && orderID != "" {
			pmt, err = h.paymentRepo.GetPaymentByOrderID(c.UserContext(), orderID)
		}
		if (err != nil || pmt == nil) && paymentID != "" && lookupID != paymentID {
			pmt, err = h.paymentRepo.GetPaymentByID(c.UserContext(), paymentID)
		}
		if (err != nil || pmt == nil) && orderID != "" && !isFailed {
			pmtAmount := cb.Amount
			if pmtAmount <= 0 {
				pmtAmount = 149.99
			}
			pmt = &model.Payment{
				ID:                   uuid.New().String(),
				OrderID:              orderID,
				UserID:               uuid.New().String(),
				PaymentMethod:        model.NormalizePaymentMethod("CARD"),
				Amount:               pmtAmount,
				Currency:             "INR",
				Status:               model.PaymentStatusPending,
				TransactionID:        fmt.Sprintf("tx_%s", uuid.New().String()),
				GatewayTransactionID: paymentID,
			}
			_ = h.paymentRepo.CreatePayment(c.UserContext(), pmt)
		}

		if pmt != nil {
			if isFailed {
				reason := errorDesc
				if reason == "" {
					reason = "Payment failed at gateway callback"
				}
				_, updateErr := h.paymentRepo.UpdatePaymentStatus(c.UserContext(), pmt.ID, model.PaymentStatusFailed, paymentID, reason)
				if updateErr != nil {
					h.log.Error("failed to update payment status to FAILED on callback", "payment_id", pmt.ID, "error", updateErr)
				} else {
					h.log.Info("updated payment status to FAILED on callback", "payment_id", pmt.ID, "reason", reason)
				}
			} else {
				_, updateErr := h.paymentRepo.UpdatePaymentStatus(c.UserContext(), pmt.ID, model.PaymentStatusSuccess, paymentID, "")
				if updateErr != nil {
					h.log.Error("failed to update payment status to SUCCESS on callback", "payment_id", pmt.ID, "error", updateErr)
				} else {
					h.log.Info("updated payment status to SUCCESS on callback", "payment_id", pmt.ID, "razorpay_payment_id", paymentID)
				}
			}
		}
	}

	responseStatus := "success"
	if isFailed {
		responseStatus = "failed"
	}

	return c.JSON(fiber.Map{
		"status":     responseStatus,
		"order_id":   orderID,
		"payment_id": paymentID,
		"signature":  signature,
	})
}

func (h *PaymentHandler) ProcessRefund(c *fiber.Ctx) error {
	if h.paymentService == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"success": false,
			"error":   "payment service unavailable",
		})
	}

	type refundReq struct {
		ReturnID     string  `json:"return_id"`
		OrderID      string  `json:"order_id"`
		OrderItemID  string  `json:"order_item_id"`
		UserID       string  `json:"user_id"`
		RefundAmount float64 `json:"refund_amount"`
		Type         string  `json:"type"`
		Reason       string  `json:"reason"`
	}

	var req refundReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "invalid request body: " + err.Error(),
		})
	}

	if req.ReturnID == "" {
		req.ReturnID = uuid.New().String()
	}
	if req.OrderItemID == "" {
		req.OrderItemID = uuid.New().String()
	}
	if req.UserID == "" {
		req.UserID = uuid.New().String()
	}
	if req.Type == "" {
		req.Type = "RETURN"
	}
	if req.Reason == "" {
		req.Reason = "Return refund initiated"
	}

	dto := &service.ProcessReturnRefundDTO{
		ReturnID:     req.ReturnID,
		OrderID:      req.OrderID,
		OrderItemID:  req.OrderItemID,
		UserID:       req.UserID,
		RefundAmount: req.RefundAmount,
		Type:         req.Type,
		Reason:       req.Reason,
	}

	refund, err := h.paymentService.ProcessReturnRefund(c.UserContext(), dto)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	var pmt *model.Payment
	var allRefunds []*model.Refund
	if h.paymentRepo != nil {
		pmt, _ = h.paymentRepo.GetPaymentByOrderID(c.UserContext(), req.OrderID)
		if pmt != nil {
			allRefunds, _ = h.paymentRepo.GetRefundsByPaymentID(c.UserContext(), pmt.ID)
		}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"refund":  refund,
		"payment": pmt,
		"refunds": allRefunds,
	})
}

func (h *PaymentHandler) GetOrderPayment(c *fiber.Ctx) error {
	orderID := c.Params("order_id")
	if orderID == "" || h.paymentRepo == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "order_id is required"})
	}

	pmt, err := h.paymentRepo.GetPaymentByOrderID(c.UserContext(), orderID)
	if err != nil || pmt == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "payment not found"})
	}

	refunds, _ := h.paymentRepo.GetRefundsByPaymentID(c.UserContext(), pmt.ID)
	return c.JSON(fiber.Map{
		"payment": pmt,
		"refunds": refunds,
	})
}

func (h *PaymentHandler) ServeTestUI(c *fiber.Ctx) error {
	paths := []string{
		"templates/razorpay_refund_test.html",
		"../../templates/razorpay_refund_test.html",
		"../templates/razorpay_refund_test.html",
	}

	for _, p := range paths {
		if content, err := os.ReadFile(p); err == nil {
			c.Set("Content-Type", "text/html; charset=utf-8")
			return c.Send(content)
		}
	}

	return c.Status(fiber.StatusNotFound).SendString("Test UI template not found on disk")
}
