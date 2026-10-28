package handler

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/middleware"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/service"
)

type DeliveryHandler struct {
	deliveryService service.DeliveryService
	jwtSecret       string
}

func NewDeliveryHandler(deliveryService service.DeliveryService, jwtSecret ...string) *DeliveryHandler {
	secret := ""
	if len(jwtSecret) > 0 {
		secret = jwtSecret[0]
	}
	return &DeliveryHandler{
		deliveryService: deliveryService,
		jwtSecret:       secret,
	}
}

func (h *DeliveryHandler) RegisterRoutes(app *fiber.App) {
	app.Get("/orders/:orderId/tracking", h.GetTrackingByOrderID)
	app.Get("/orders/:orderId/estimate", h.EstimateDelivery)
	app.Get("/api/deliveries/estimate/:orderId", h.EstimateDelivery)
	app.Get("/deliveries/:trackingNumber", h.GetTrackingByNumber)
	app.Patch("/deliveries/:id/status", h.UpdateStatus)
	app.Post("/webhooks/delivery", h.HandleWebhook)
	app.Post("/api/webhooks/delivery", h.HandleWebhook)

	// Partner Profile Management
	var partnerAuth fiber.Handler
	if h.jwtSecret != "" {
		partnerAuth = middleware.DeliveryPartnerAuthMiddleware(h.jwtSecret)
	} else {
		partnerAuth = func(c *fiber.Ctx) error { return c.Next() }
	}

	app.Get("/api/partners/me", partnerAuth, h.GetMyProfile)
	app.Put("/api/partners/me", partnerAuth, h.UpdateMyProfile)
}

func (h *DeliveryHandler) GetTrackingByOrderID(c *fiber.Ctx) error {
	orderID := strings.TrimSpace(c.Params("orderId"))
	if orderID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeBadRequest,
				"message": "order_id is required",
			},
		})
	}

	delivery, err := h.deliveryService.GetDeliveryByOrderID(c.UserContext(), orderID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(dto.FromDelivery(delivery))
}

func (h *DeliveryHandler) GetTrackingByNumber(c *fiber.Ctx) error {
	trackingNumber := strings.TrimSpace(c.Params("trackingNumber"))
	if trackingNumber == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeBadRequest,
				"message": "tracking_number is required",
			},
		})
	}

	delivery, err := h.deliveryService.GetDeliveryByTrackingNumber(c.UserContext(), trackingNumber)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(dto.FromDelivery(delivery))
}

func (h *DeliveryHandler) UpdateStatus(c *fiber.Ctx) error {
	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeBadRequest,
				"message": "id is required",
			},
		})
	}

	var req dto.UpdateStatusRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeBadRequest,
				"message": "invalid request body",
			},
		})
	}

	targetStatus := model.DeliveryStatus(strings.ToUpper(strings.TrimSpace(string(req.Status))))
	if targetStatus == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeBadRequest,
				"message": "status is required",
			},
		})
	}

	delivery, err := h.deliveryService.ProcessStatusUpdate(c.UserContext(), &dto.DeliveryStatusUpdateRequest{
		DeliveryID:        id,
		Status:            targetStatus,
		EstimatedDelivery: req.EstimatedDelivery,
		ActualDelivery:    req.ActualDelivery,
		Location:          req.Location,
		Notes:             req.Notes,
	})
	if err != nil {
		return h.handleError(c, err)
	}

	resp := fiber.Map{
		"id":               delivery.ID,
		"order_id":         delivery.OrderID,
		"tracking_number":  delivery.TrackingNumber,
		"delivery_partner": delivery.DeliveryPartner,
		"status":           delivery.Status,
		"shipping_address": delivery.ShippingAddress,
		"updated_at":       delivery.UpdatedAt,
	}
	if delivery.EstimatedDelivery != nil {
		resp["estimated_delivery"] = delivery.EstimatedDelivery
	}
	if delivery.ActualDelivery != nil {
		resp["actual_delivery"] = delivery.ActualDelivery
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

func (h *DeliveryHandler) HandleWebhook(c *fiber.Ctx) error {
	var req dto.DeliveryStatusUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeBadRequest,
				"message": "invalid request body",
			},
		})
	}

	delivery, err := h.deliveryService.ProcessStatusUpdate(c.UserContext(), &req)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(dto.FromDelivery(delivery))
}

func (h *DeliveryHandler) EstimateDelivery(c *fiber.Ctx) error {
	orderID := strings.TrimSpace(c.Params("orderId"))
	if orderID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeBadRequest,
				"message": "order_id is required",
			},
		})
	}

	partner := strings.TrimSpace(c.Query("partner"))
	estimate, err := h.deliveryService.EstimateDelivery(c.UserContext(), orderID, partner)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(estimate)
}

func (h *DeliveryHandler) GetMyProfile(c *fiber.Ctx) error {
	partnerID := middleware.GetPartnerID(c)
	if partnerID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeUnauthorized,
				"message": "partner_id not found in request context",
			},
		})
	}

	profile, err := h.deliveryService.GetPartnerProfile(c.UserContext(), partnerID)
	if err != nil {
		return h.handleError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(dto.FromPartnerProfile(profile))
}

func (h *DeliveryHandler) UpdateMyProfile(c *fiber.Ctx) error {
	partnerID := middleware.GetPartnerID(c)
	if partnerID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeUnauthorized,
				"message": "partner_id not found in request context",
			},
		})
	}

	var req dto.UpdatePartnerProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeBadRequest,
				"message": "invalid request body",
			},
		})
	}

	profile, err := h.deliveryService.UpdatePartnerProfile(c.UserContext(), partnerID, req.CompanyName, req.Email, req.ContactNumber)
	if err != nil {
		return h.handleError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(dto.FromPartnerProfile(profile))
}

func (h *DeliveryHandler) handleError(c *fiber.Ctx, err error) error {
	var transitionErr *model.InvalidStateTransitionError
	if errors.As(err, &transitionErr) {
		return c.Status(transitionErr.HTTPStatusCode()).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeUnprocessableEntity,
				"message": transitionErr.Error(),
			},
		})
	}

	appErr := appErrors.AsAppError(err)
	if appErr != nil {
		statusCode := appErr.HTTPStatus
		if statusCode == 0 {
			statusCode = fiber.StatusInternalServerError
		}
		return c.Status(statusCode).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErr.Code,
				"message": appErr.Message,
			},
		})
	}

	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"error": fiber.Map{
			"code":    appErrors.CodeInternalError,
			"message": err.Error(),
		},
	})
}
