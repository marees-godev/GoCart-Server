package handler

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
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
	app.Post("/webhooks", h.HandleRazorpayWebhook)
}

func (h *WebhookHandler) HandleRazorpayWebhook(c *fiber.Ctx) error {
	ctx := c.UserContext()
	body := c.Body()

	signature := c.Get("X-Razorpay-Signature")
	if signature == "" {
		signature = c.Get("x-razorpay-signature")
	}

	slog.InfoContext(ctx, "received HTTP razorpay webhook request", "body_len", len(body), "has_signature", signature != "")

	if signature == "" || h.webhookSecret == "" || !gateway.VerifyRazorpayWebhookSignature(body, signature, h.webhookSecret) {
		slog.WarnContext(ctx, "razorpay webhook signature verification failed")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "invalid razorpay webhook signature",
		})
	}

	if err := h.paymentService.HandleRazorpayWebhook(ctx, body); err != nil {
		slog.ErrorContext(ctx, "failed to process razorpay webhook event in handler", "error", err)
		if appErrors.IsNotFound(err) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"status":  "error",
				"message": err.Error(),
			})
		}
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
