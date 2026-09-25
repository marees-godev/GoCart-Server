package handler

import (
	"log/slog"
	"strings"

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
	ctx := c.UserContext()
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		slog.WarnContext(ctx, "missing authenticated user context in HTTP GetMyProfile")
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}
	if !dto.IsValidID(authUserID) {
		slog.WarnContext(ctx, "invalid authenticated user ID in HTTP GetMyProfile", "user_id", authUserID)
		err := errors.BadRequest("user_id is invalid")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	user, err := h.userService.GetUser(c.Context(), authUserID, authUserID)
	if err != nil {
		slog.WarnContext(ctx, "service error in HTTP GetMyProfile", "user_id", authUserID, "error", err)
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	slog.InfoContext(ctx, "HTTP GetMyProfile succeeded", "user_id", user.ID)
	return c.JSON(dto.ToUserResponse(user))
}

func (h *UserHandler) GetUserByID(c *fiber.Ctx) error {
	ctx := c.UserContext()
	id := c.Params("id")
	if strings.TrimSpace(id) == "" {
		slog.WarnContext(ctx, "missing user id in HTTP GetUserByID")
		err := errors.BadRequest("user_id is required")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}
	if !dto.IsValidID(id) {
		slog.WarnContext(ctx, "invalid user id in HTTP GetUserByID", "user_id", id)
		err := errors.BadRequest("user_id is invalid")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	user, err := h.userService.GetUserByID(c.Context(), id)
	if err != nil {
		slog.WarnContext(ctx, "service error in HTTP GetUserByID", "user_id", id, "error", err)
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	slog.InfoContext(ctx, "HTTP GetUserByID succeeded", "user_id", user.ID)
	return c.JSON(dto.ToUserResponse(user))
}

func (h *UserHandler) UpdateProfile(c *fiber.Ctx) error {
	ctx := c.UserContext()
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		slog.WarnContext(ctx, "missing authenticated user context in HTTP UpdateProfile")
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	targetID := c.Params("id")
	if strings.TrimSpace(targetID) == "" {
		targetID = authUserID
	}

	if !dto.IsValidID(targetID) {
		slog.WarnContext(ctx, "invalid target id in HTTP UpdateProfile", "target_id", targetID)
		err := errors.BadRequest("user_id is invalid")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	if authUserID != targetID {
		slog.WarnContext(ctx, "user id mismatch in HTTP UpdateProfile", "auth_user_id", authUserID, "target_id", targetID)
		err := errors.BadRequest("user_id does not match the expected value")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	var req dto.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		slog.WarnContext(ctx, "invalid request payload in HTTP UpdateProfile", "error", err)
		appErr := errors.BadRequest("invalid request payload")
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	user, err := h.userService.UpdateUser(c.Context(), authUserID, targetID, req)
	if err != nil {
		slog.WarnContext(ctx, "service error in HTTP UpdateProfile", "target_id", targetID, "error", err)
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	slog.InfoContext(ctx, "HTTP UpdateProfile succeeded", "user_id", user.ID)
	return c.JSON(dto.ToUserResponse(user))
}

func (h *UserHandler) DeactivateAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		slog.WarnContext(ctx, "missing authenticated user context in HTTP DeactivateAccount")
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	targetID := c.Params("id")
	if targetID != "" && targetID != authUserID {
		slog.WarnContext(ctx, "user id mismatch in HTTP DeactivateAccount", "auth_user_id", authUserID, "target_id", targetID)
		err := errors.BadRequest("user_id does not match the expected value")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	var req dto.DeactivateUserRequest
	_ = c.BodyParser(&req)

	res, err := h.userService.DeactivateUser(c.Context(), authUserID, authUserID, req)
	if err != nil {
		slog.WarnContext(ctx, "service error in HTTP DeactivateAccount", "user_id", authUserID, "error", err)
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	slog.InfoContext(ctx, "HTTP DeactivateAccount succeeded", "user_id", authUserID)
	return c.JSON(res)
}

func (h *UserHandler) DeleteAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		slog.WarnContext(ctx, "missing authenticated user context in HTTP DeleteAccount")
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	targetID := c.Params("id")
	if targetID != "" && targetID != authUserID {
		slog.WarnContext(ctx, "user id mismatch in HTTP DeleteAccount", "auth_user_id", authUserID, "target_id", targetID)
		err := errors.BadRequest("user_id does not match the expected value")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	var req dto.DeleteUserRequest
	_ = c.BodyParser(&req)

	res, err := h.userService.DeleteUser(c.Context(), authUserID, authUserID, req)
	if err != nil {
		slog.WarnContext(ctx, "service error in HTTP DeleteAccount", "user_id", authUserID, "error", err)
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	slog.InfoContext(ctx, "HTTP DeleteAccount succeeded", "user_id", authUserID)
	return c.JSON(res)
}

func (h *UserHandler) ReactivateAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		slog.WarnContext(ctx, "missing authenticated user context in HTTP ReactivateAccount")
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	targetID := c.Params("id")
	if targetID != "" && targetID != authUserID {
		slog.WarnContext(ctx, "user id mismatch in HTTP ReactivateAccount", "auth_user_id", authUserID, "target_id", targetID)
		err := errors.BadRequest("user_id does not match the expected value")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	res, err := h.userService.ReactivateUser(c.Context(), authUserID, authUserID)
	if err != nil {
		slog.WarnContext(ctx, "service error in HTTP ReactivateAccount", "user_id", authUserID, "error", err)
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	slog.InfoContext(ctx, "HTTP ReactivateAccount succeeded", "user_id", authUserID)
	return c.JSON(res)
}
