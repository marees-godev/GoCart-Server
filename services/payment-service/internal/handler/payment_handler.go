package handler

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/gateway"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/repository"
)

type PaymentHandler struct {
	paymentRepo repository.PaymentRepository
	paymentGW   gateway.PaymentGateway
	gwConfig    gateway.GatewayConfig
	log         *slog.Logger
}

func NewPaymentHandler(paymentRepo repository.PaymentRepository, paymentGW gateway.PaymentGateway, gwConfig gateway.GatewayConfig, log *slog.Logger) *PaymentHandler {
	if log == nil {
		log = slog.Default()
	}
	return &PaymentHandler{
		paymentRepo: paymentRepo,
		paymentGW:   paymentGW,
		gwConfig:    gwConfig,
		log:         log,
	}
}

func (h *PaymentHandler) RegisterRoutes(app *fiber.App) {
	app.Get("/get-key", h.GetKey)
	app.Post("/create-order", h.CreateOrder)
	app.All("/payment-callback", h.PaymentCallback)
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
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	}
	var req orderReq
	_ = c.BodyParser(&req)
	if req.Amount <= 0 {
		req.Amount = 1499
	}
	if req.Currency == "" {
		req.Currency = "INR"
	}

	// If real Razorpay key & secret are provided, create genuine Razorpay Order ID via API
	if h.gwConfig.RazorpayKeyID != "" && h.gwConfig.RazorpayKeySecret != "" && !strings.Contains(h.gwConfig.RazorpayKeyID, "xxxx") {
		paymentID := uuid.New().String()
		orderID := uuid.New().String()
		userID := uuid.New().String()
		idempKey := fmt.Sprintf("idemp_%d", time.Now().UnixNano())

		rzpResp, err := h.paymentGW.ProcessPayment(c.UserContext(), &gateway.ProcessGatewayRequest{
			PaymentID:      paymentID,
			OrderID:        orderID,
			Amount:         req.Amount,
			Currency:       req.Currency,
			IdempotencyKey: idempKey,
		})

		if err == nil && rzpResp != nil && rzpResp.GatewayTransactionID != "" {
			pmt := &model.Payment{
				ID:                   paymentID,
				OrderID:              orderID,
				UserID:               userID,
				PaymentMethod:        "CREDIT_CARD",
				Amount:               req.Amount,
				Currency:             req.Currency,
				Status:               model.PaymentStatusPending,
				TransactionID:        fmt.Sprintf("tx_%s", uuid.New().String()),
				GatewayTransactionID: rzpResp.GatewayTransactionID,
				IdempotencyKey:       idempKey,
			}
			if createErr := h.paymentRepo.CreatePayment(c.UserContext(), pmt); createErr != nil {
				h.log.Error("failed to persist payment record for razorpay order", "error", createErr)
			} else {
				h.log.Info("persisted pending razorpay payment in database", "payment_id", pmt.ID, "razorpay_order_id", rzpResp.GatewayTransactionID)
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
	orderID := c.FormValue("razorpay_order_id", c.Query("razorpay_order_id", c.Query("orderId", "")))
	paymentID := c.FormValue("razorpay_payment_id", c.Query("razorpay_payment_id", c.Query("paymentId", "")))
	signature := c.FormValue("razorpay_signature", c.Query("razorpay_signature", c.Query("signature", "")))

	return c.JSON(fiber.Map{
		"status":     "success",
		"order_id":   orderID,
		"payment_id": paymentID,
		"signature":  signature,
	})
}
