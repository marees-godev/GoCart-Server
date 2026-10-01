package rest

import (
	"crypto/subtle"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type UpdateMerchantStatusPayload struct {
	Status *string `json:"status"`
	Reason string  `json:"reason"`
}

type UpdateMerchantProfilePayload struct {
	BusinessName  *string `json:"businessName"`
	FirstName     *string `json:"firstName"`
	LastName      *string `json:"lastName"`
	BusinessPhone *string `json:"businessPhone"`
	PanCardNumber *string `json:"panCardNumber"`
	Status        *string `json:"status,omitempty"`
}

type MerchantResponseDTO struct {
	MerchantID      string  `json:"merchantId"`
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	BusinessName    string  `json:"businessName"`
	FirstName       string  `json:"firstName"`
	LastName        string  `json:"lastName"`
	BusinessEmail   string  `json:"businessEmail"`
	BusinessPhone   string  `json:"businessPhone"`
	PanCardNumber   string  `json:"panCardNumber"`
	Status          string  `json:"status"`
	RejectionReason string  `json:"rejectionReason,omitempty"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
	PreviousStatus  *string `json:"previousStatus,omitempty"`
}

type MerchantListResponseDTO struct {
	Merchants []*MerchantResponseDTO `json:"merchants"`
	Total     int                    `json:"total"`
}

func formatMerchantName(m *merchantpb.MerchantResponseData) string {
	if m == nil {
		return ""
	}
	if strings.TrimSpace(m.BusinessName) != "" {
		return strings.TrimSpace(m.BusinessName)
	}
	name := strings.TrimSpace(m.FirstName + " " + m.LastName)
	if name != "" {
		return name
	}
	return m.Id
}

func toMerchantDTO(m *merchantpb.MerchantResponseData, prevStatus *string) *MerchantResponseDTO {
	if m == nil {
		return nil
	}
	createdAt := ""
	if m.CreatedAt != nil {
		createdAt = m.CreatedAt.AsTime().Format(time.RFC3339)
	}
	updatedAt := ""
	if m.UpdatedAt != nil {
		updatedAt = m.UpdatedAt.AsTime().Format(time.RFC3339)
	}

	return &MerchantResponseDTO{
		MerchantID:      m.Id,
		ID:              m.Id,
		Name:            formatMerchantName(m),
		BusinessName:    m.BusinessName,
		FirstName:       m.FirstName,
		LastName:        m.LastName,
		BusinessEmail:   m.BusinessEmail,
		BusinessPhone:   m.BusinessPhone,
		PanCardNumber:   m.PanCardNumber,
		Status:          m.Status.String(),
		RejectionReason: m.RejectionReason,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		PreviousStatus:  prevStatus,
	}
}

func AdminKeyGuard(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := c.Get("adminkey")
		if key == "" {
			key = c.Get("X-Admin-Key")
		}
		if key == "" {
			key = c.Get("Admin-Key")
		}

		expectedKey := ""
		if cfg != nil {
			expectedKey = cfg.AdminAPIKey
		}

		if key == "" || expectedKey == "" || subtle.ConstantTimeCompare([]byte(key), []byte(expectedKey)) != 1 {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "UNAUTHORIZED",
				"message": "Missing, blank, or invalid adminkey header.",
			})
		}

		return c.Next()
	}
}

func RegisterMerchantRoutes(app *fiber.App, merchantClient merchantpb.MerchantServiceClient, cfg *config.Config) {
	if merchantClient == nil {
		return
	}

	adminGuard := AdminKeyGuard(cfg)

	// Administrative status update handler
	updateStatusHandler := func(c *fiber.Ctx) error {
		merchantID := strings.TrimSpace(c.Params("merchantId"))
		if merchantID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "BAD_REQUEST",
				"message": "Missing merchant ID.",
			})
		}

		body := c.Body()
		if len(body) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "BAD_REQUEST",
				"message": "Missing request body.",
			})
		}

		var payload UpdateMerchantStatusPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "BAD_REQUEST",
				"message": "Invalid JSON payload.",
			})
		}

		if payload.Status == nil || strings.TrimSpace(*payload.Status) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "BAD_REQUEST",
				"message": "Missing status field.",
			})
		}

		statusVal := strings.ToUpper(strings.TrimSpace(*payload.Status))
		switch statusVal {
		case "PENDING", "APPROVED", "REJECTED", "SUSPENDED":
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "BAD_REQUEST",
				"message": "Unknown enum value '" + *payload.Status + "'. Allowed values: ['PENDING', 'APPROVED', 'REJECTED', 'SUSPENDED'].",
			})
		}

		ctx := metadata.AppendToOutgoingContext(c.UserContext(),
			"x-user-role", "ADMIN",
			"x-user-id", "admin",
		)

		statusEnum := merchantpb.MerchantStatus(merchantpb.MerchantStatus_value[statusVal])

		req := &merchantpb.UpdateMerchantStatusRequest{
			Id:              merchantID,
			Status:          statusEnum,
			RejectionReason: strings.TrimSpace(payload.Reason),
		}

		res, err := merchantClient.UpdateMerchantStatus(ctx, req)
		if err != nil {
			st, ok := status.FromError(err)
			if ok {
				switch st.Code() {
				case codes.FailedPrecondition:
					return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
						"error":   "INVALID_STATE_TRANSITION",
						"message": st.Message(),
					})
				case codes.NotFound:
					return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
						"error":   "NOT_FOUND",
						"message": "Merchant ID not found.",
					})
				case codes.InvalidArgument:
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
						"error":   "BAD_REQUEST",
						"message": st.Message(),
					})
				case codes.PermissionDenied:
					return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
						"error":   "FORBIDDEN",
						"message": st.Message(),
					})
				}
			}

			errMsg := err.Error()
			if strings.Contains(errMsg, "Cannot transition merchant") {
				return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
					"error":   "INVALID_STATE_TRANSITION",
					"message": errMsg,
				})
			}
			if strings.Contains(strings.ToLower(errMsg), "not found") {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error":   "NOT_FOUND",
					"message": "Merchant ID not found.",
				})
			}

			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Failed to update merchant status.",
			})
		}

		if res == nil || res.Merchant == nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Empty response from merchant service.",
			})
		}

		updatedAtStr := time.Now().UTC().Format(time.RFC3339)
		if res.Merchant.UpdatedAt != nil {
			updatedAtStr = res.Merchant.UpdatedAt.AsTime().Format(time.RFC3339)
		}

		prevStatus := res.PreviousStatus.String()
		if res.PreviousStatus.String() == "" {
			prevStatus = "PENDING"
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"merchantId":     res.Merchant.Id,
			"name":           formatMerchantName(res.Merchant),
			"status":         res.Merchant.Status.String(),
			"previousStatus": prevStatus,
			"updatedAt":      updatedAtStr,
		})
	}

	// 1. Guarded Administrative Status Update Endpoints (PATCH and PUT)
	app.Patch("/api/v1/admin/merchants/:merchantId/status", adminGuard, updateStatusHandler)
	app.Put("/api/v1/admin/merchants/:merchantId/status", adminGuard, updateStatusHandler)

	// 2. Merchant Lookup Endpoints (GET /api/v1/merchants/{merchantId} and GET /api/v1/admin/merchants/{merchantId})
	getMerchantHandler := func(c *fiber.Ctx) error {
		merchantID := strings.TrimSpace(c.Params("merchantId"))
		if merchantID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "BAD_REQUEST",
				"message": "Missing merchant ID.",
			})
		}

		ctx := metadata.AppendToOutgoingContext(c.UserContext(),
			"x-user-role", "ADMIN",
			"x-user-id", "admin",
		)

		res, err := merchantClient.GetMerchant(ctx, &merchantpb.GetMerchantRequest{Id: merchantID})
		if err != nil {
			st, ok := status.FromError(err)
			if ok && st.Code() == codes.NotFound {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error":   "NOT_FOUND",
					"message": "Merchant ID not found.",
				})
			}
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error":   "NOT_FOUND",
					"message": "Merchant ID not found.",
				})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Failed to get merchant.",
			})
		}

		return c.Status(fiber.StatusOK).JSON(toMerchantDTO(res.Merchant, nil))
	}

	app.Get("/api/v1/merchants/:merchantId", getMerchantHandler)
	app.Get("/api/v1/admin/merchants/:merchantId", adminGuard, getMerchantHandler)

	// 3. Merchant List Query Endpoints (GET /api/v1/merchants and GET /api/v1/admin/merchants)
	listMerchantsHandler := func(c *fiber.Ctx) error {
		statusFilter := strings.TrimSpace(c.Query("status"))
		limit := c.QueryInt("limit", 20)
		offset := c.QueryInt("offset", 0)

		ctx := metadata.AppendToOutgoingContext(c.UserContext(),
			"x-user-role", "ADMIN",
			"x-user-id", "admin",
		)

		req := &merchantpb.ListMerchantsRequest{
			Limit:  int32(limit),
			Offset: int32(offset),
		}
		if statusFilter != "" {
			if val, ok := merchantpb.MerchantStatus_value[strings.ToUpper(statusFilter)]; ok {
				st := merchantpb.MerchantStatus(val)
				req.Status = &st
			}
		}

		res, err := merchantClient.ListMerchants(ctx, req)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Failed to list merchants.",
			})
		}

		dtos := make([]*MerchantResponseDTO, len(res.Merchants))
		for i, m := range res.Merchants {
			dtos[i] = toMerchantDTO(m, nil)
		}

		return c.Status(fiber.StatusOK).JSON(MerchantListResponseDTO{
			Merchants: dtos,
			Total:     int(res.Total),
		})
	}

	app.Get("/api/v1/merchants", listMerchantsHandler)
	app.Get("/api/v1/admin/merchants", adminGuard, listMerchantsHandler)

	// 4. Generic Profile Update Endpoint (Disallowing Status Modification)
	updateProfileHandler := func(c *fiber.Ctx) error {
		merchantID := strings.TrimSpace(c.Params("merchantId"))
		if merchantID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "BAD_REQUEST",
				"message": "Missing merchant ID.",
			})
		}

		var payload UpdateMerchantProfilePayload
		if err := json.Unmarshal(c.Body(), &payload); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "BAD_REQUEST",
				"message": "Invalid JSON payload.",
			})
		}

		if payload.Status != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "BAD_REQUEST",
				"message": "Modifying status via profile update endpoint is prohibited. Use administrative status update endpoint.",
			})
		}

		ctx := metadata.AppendToOutgoingContext(c.UserContext(),
			"x-user-role", "ADMIN",
			"x-user-id", "admin",
		)

		req := &merchantpb.UpdateMerchantRequest{
			Id: merchantID,
		}
		if payload.BusinessName != nil {
			req.BusinessName = *payload.BusinessName
		}
		if payload.FirstName != nil {
			req.FirstName = *payload.FirstName
		}
		if payload.LastName != nil {
			req.LastName = *payload.LastName
		}
		if payload.BusinessPhone != nil {
			req.BusinessPhone = *payload.BusinessPhone
		}
		if payload.PanCardNumber != nil {
			req.PanCardNumber = *payload.PanCardNumber
		}

		res, err := merchantClient.UpdateMerchant(ctx, req)
		if err != nil {
			st, ok := status.FromError(err)
			if ok && st.Code() == codes.NotFound {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error":   "NOT_FOUND",
					"message": "Merchant ID not found.",
				})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "INTERNAL_SERVER_ERROR",
				"message": "Failed to update merchant profile.",
			})
		}

		return c.Status(fiber.StatusOK).JSON(toMerchantDTO(res.Merchant, nil))
	}

	app.Put("/api/v1/merchants/:merchantId", updateProfileHandler)
}
