package handler

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/gateway"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/service"
)

type WebhookHandler struct {
	paymentService service.PaymentService
	webhookSecret  string
}

func NewWebhookHandler(paymentService service.PaymentService, webhookSecret string) *WebhookHandler {
	return &WebhookHandler{
		paymentService: paymentService,
		webhookSecret:  webhookSecret,
	}
}

func (h *WebhookHandler) RegisterRoutes(app *fiber.App) {
	app.Post("/webhooks/razorpay", h.HandleRazorpayWebhook)
	app.Post("/webhooks/payment/razorpay", h.HandleRazorpayWebhook)
	app.Post("/api/v1/webhooks/razorpay", h.HandleRazorpayWebhook)
}

func (h *WebhookHandler) HandleRazorpayWebhook(c *fiber.Ctx) error {
	ctx := c.UserContext()
	body := c.Body()

	signature := c.Get("X-Razorpay-Signature")
	if signature == "" {
		signature = c.Get("x-razorpay-signature")
	}

	slog.InfoContext(ctx, "received HTTP razorpay webhook request", "body_len", len(body), "has_signature", signature != "")

	if h.webhookSecret != "" {
		if signature == "" || !gateway.VerifyRazorpayWebhookSignature(body, signature, h.webhookSecret) {
			slog.WarnContext(ctx, "razorpay webhook signature verification failed", "provided_signature", signature)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"status":  "error",
				"message": "invalid razorpay webhook signature",
			})
		}
	}

	if err := h.paymentService.HandleRazorpayWebhook(ctx, body); err != nil {
		slog.ErrorContext(ctx, "failed to process razorpay webhook event in handler", "error", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":  "success",
		"message": "webhook processed successfully",
	})
}
