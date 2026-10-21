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
	app.Get("/deliveries/:trackingNumber", h.GetTrackingByNumber)
	app.Patch("/deliveries/:id/status", h.UpdateStatus)

	// Warehouse Location Lookup API (Task 6)
	app.Get("/api/warehouses/available", h.GetAvailableWarehouses)

	// Partner Profile & Warehouse Association (Tasks 7, 8)
	var partnerAuth fiber.Handler
	if h.jwtSecret != "" {
		partnerAuth = middleware.DeliveryPartnerAuthMiddleware(h.jwtSecret)
	} else {
		partnerAuth = func(c *fiber.Ctx) error { return c.Next() }
	}

	app.Post("/api/partners/warehouses", partnerAuth, h.SelectWarehouses)
	app.Get("/api/partners/me/warehouses", partnerAuth, h.GetMyWarehouses)
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

	delivery, err := h.deliveryService.UpdateStatus(c.UserContext(), id, targetStatus)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"id":               delivery.ID,
		"order_id":         delivery.OrderID,
		"tracking_number":  delivery.TrackingNumber,
		"delivery_partner": delivery.DeliveryPartner,
		"status":           delivery.Status,
		"shipping_address": delivery.ShippingAddress,
		"updated_at":       delivery.UpdatedAt,
	})
}

func (h *DeliveryHandler) GetAvailableWarehouses(c *fiber.Ctx) error {
	warehouses, err := h.deliveryService.GetAvailableWarehouses(c.UserContext())
	if err != nil {
		return h.handleError(c, err)
	}
	resp := make([]*dto.WarehouseResponse, len(warehouses))
	for i, w := range warehouses {
		resp[i] = dto.FromWarehouse(w)
	}
	return c.Status(fiber.StatusOK).JSON(resp)
}

func (h *DeliveryHandler) SelectWarehouses(c *fiber.Ctx) error {
	partnerID := middleware.GetPartnerID(c)
	if partnerID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeUnauthorized,
				"message": "partner_id not found in request context",
			},
		})
	}

	var req dto.AssignWarehousesRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeBadRequest,
				"message": "invalid request body",
			},
		})
	}

	if err := h.deliveryService.AssignPartnerWarehouses(c.UserContext(), partnerID, req.WarehouseIDs); err != nil {
		return h.handleError(c, err)
	}

	warehouses, err := h.deliveryService.GetPartnerWarehouses(c.UserContext(), partnerID)
	if err != nil {
		return h.handleError(c, err)
	}
	resp := make([]*dto.WarehouseResponse, len(warehouses))
	for i, w := range warehouses {
		resp[i] = dto.FromWarehouse(w)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message":    "warehouses associated successfully",
		"warehouses": resp,
	})
}

func (h *DeliveryHandler) GetMyWarehouses(c *fiber.Ctx) error {
	partnerID := middleware.GetPartnerID(c)
	if partnerID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    appErrors.CodeUnauthorized,
				"message": "partner_id not found in request context",
			},
		})
	}

	warehouses, err := h.deliveryService.GetPartnerWarehouses(c.UserContext(), partnerID)
	if err != nil {
		return h.handleError(c, err)
	}
	resp := make([]*dto.WarehouseResponse, len(warehouses))
	for i, w := range warehouses {
		resp[i] = dto.FromWarehouse(w)
	}
	return c.Status(fiber.StatusOK).JSON(resp)
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
