package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/service"
)

type AuthHandler struct {
	authService service.AuthService
}

func NewAuthHandler(authService service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

func (h *AuthHandler) RegisterRoutes(r fiber.Router) {
	auth := r.Group("/auth")
	auth.Post("/register", h.Register)
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req dto.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		appErr := errors.BadRequest("Invalid request payload format")
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	resp, err := h.authService.Register(c.Context(), req)
	if err != nil {
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	return c.Status(fiber.StatusCreated).JSON(resp)
}
