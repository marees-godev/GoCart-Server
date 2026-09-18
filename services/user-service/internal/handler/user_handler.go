package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/service"
)

type UserHandler struct {
	userService service.UserService
}

func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

func (h *UserHandler) RegisterRoutes(router fiber.Router) {
	users := router.Group("/api/v1/users")
	users.Get("/me", h.GetMyProfile)
	users.Get("/:id", h.GetUserByID)
	users.Put("/:id", h.UpdateProfile)
}

func getAuthUserID(c *fiber.Ctx) string {
	if user, ok := auth.UserFromContext(c.UserContext()); ok && user != nil {
		return user.UserID
	}
	if userID := c.Get("X-User-ID"); userID != "" {
		return userID
	}
	return ""
}

func (h *UserHandler) GetMyProfile(c *fiber.Ctx) error {
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	user, err := h.userService.GetUser(c.Context(), authUserID, authUserID)
	if err != nil {
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	return c.JSON(dto.ToUserResponse(user))
}

func (h *UserHandler) GetUserByID(c *fiber.Ctx) error {
	id := c.Params("id")
	user, err := h.userService.GetUserByID(c.Context(), id)
	if err != nil {
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	return c.JSON(dto.ToUserResponse(user))
}

func (h *UserHandler) UpdateProfile(c *fiber.Ctx) error {
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	targetID := c.Params("id")
	var req dto.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		appErr := errors.BadRequest("invalid request payload")
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	user, err := h.userService.UpdateUser(c.Context(), authUserID, targetID, req)
	if err != nil {
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	return c.JSON(dto.ToUserResponse(user))
}
