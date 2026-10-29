package middleware

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

type contextKey string

const (
	PartnerIDKey contextKey = "partner_id"
	RoleKey      contextKey = "role"
	UserIDKey    contextKey = "user_id"
)

// DeliveryPartnerAuthMiddleware enforces JWT verification, restricts to role == "DELIVERY_PARTNER",
// and injects the verified partner_id into the request context.
// Never bypasses authentication: if the JWT secret is missing, it rejects requests with an error.
func DeliveryPartnerAuthMiddleware(jwtSecret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if strings.TrimSpace(jwtSecret) == "" {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    appErrors.CodeInternalError,
					"message": "authentication cannot be configured: JWT secret is missing",
				},
			})
		}

		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    appErrors.CodeUnauthorized,
					"message": "authorization header is required",
				},
			})
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    appErrors.CodeUnauthorized,
					"message": "invalid authorization format, expected Bearer <token>",
				},
			})
		}

		tokenStr := strings.TrimSpace(parts[1])
		claims, err := auth.ValidateToken(tokenStr, jwtSecret)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    appErrors.CodeUnauthorized,
					"message": "invalid or expired token",
				},
			})
		}

		// Strictly restrict to DELIVERY_PARTNER or ADMIN role
		role := strings.ToUpper(strings.TrimSpace(claims.Role))
		if role != "DELIVERY_PARTNER" && role != "ADMIN" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    appErrors.CodeForbidden,
					"message": "access restricted strictly to delivery partners",
				},
			})
		}

		partnerID := strings.TrimSpace(claims.PartnerID)
		if partnerID == "" {
			partnerID = strings.TrimSpace(claims.UserID)
		}

		if partnerID == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    appErrors.CodeUnauthorized,
					"message": "partner_id not found in token claims",
				},
			})
		}

		// Inject verified partner_id into Fiber locals and context
		c.Locals("partner_id", partnerID)
		c.Locals("role", claims.Role)
		c.Locals("user_id", claims.UserID)

		ctx := context.WithValue(c.UserContext(), PartnerIDKey, partnerID)
		ctx = context.WithValue(ctx, RoleKey, claims.Role)
		ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
		c.SetUserContext(ctx)

		return c.Next()
	}
}

func GetPartnerID(c *fiber.Ctx) string {
	if p, ok := c.Locals("partner_id").(string); ok && p != "" {
		return p
	}
	if p, ok := c.UserContext().Value(PartnerIDKey).(string); ok && p != "" {
		return p
	}
	return ""
}
