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
	app.Get("/", h.RenderCheckout)
	app.Get("/checkout", h.RenderCheckout)
	app.Get("/get-key", h.GetKey)
	app.Post("/create-order", h.CreateOrder)
	app.All("/payment-callback", h.PaymentCallback)
}

func (h *PaymentHandler) RenderCheckout(c *fiber.Ctx) error {
	c.Type("html")
	content, err := os.ReadFile("./templates/checkout.html")
	if err != nil {
		content, err = os.ReadFile("../templates/checkout.html")
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Checkout template not found")
	}
	return c.Send(content)
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
	orderID := c.FormValue("razorpay_order_id", c.Query("razorpay_order_id", c.Query("orderId", "")))
	paymentID := c.FormValue("razorpay_payment_id", c.Query("razorpay_payment_id", c.Query("paymentId", "")))
	signature := c.FormValue("razorpay_signature", c.Query("razorpay_signature", c.Query("signature", "")))
	statusParam := strings.ToLower(c.FormValue("status", c.Query("status", "")))
	errorCode := c.FormValue("error_code", c.Query("error_code", c.Query("error[code]", "")))
	errorDesc := c.FormValue("error_description", c.Query("error_description", c.Query("error[description]", c.Query("error_reason", c.Query("error", "")))))

	isFailed := statusParam == "failed" || statusParam == "failure" || statusParam == "error" || errorCode != ""

	if (orderID != "" || paymentID != "") && h.paymentRepo != nil {
		lookupID := orderID
		if lookupID == "" {
			lookupID = paymentID
		}
		pmt, err := h.paymentRepo.GetPaymentByID(c.UserContext(), lookupID)
		if err == nil && pmt != nil {
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
