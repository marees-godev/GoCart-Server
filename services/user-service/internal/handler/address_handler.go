package handler

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/service"
)

type AddressHandler struct {
	addressService service.AddressService
}

func NewAddressHandler(addressService service.AddressService) *AddressHandler {
	return &AddressHandler{addressService: addressService}
}

func (h *AddressHandler) CreateAddress(c *fiber.Ctx) error {
	ctx := c.UserContext()
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		slog.WarnContext(ctx, "missing authenticated user context in HTTP CreateAddress")
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	var req dto.CreateAddressRequest
	if err := c.BodyParser(&req); err != nil {
		slog.WarnContext(ctx, "invalid request payload in HTTP CreateAddress", "error", err)
		appErr := errors.BadRequest("invalid request payload")
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	addr, err := h.addressService.CreateAddress(c.Context(), authUserID, req)
	if err != nil {
		slog.WarnContext(ctx, "service error in HTTP CreateAddress", "error", err)
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	slog.InfoContext(ctx, "HTTP CreateAddress succeeded", "address_id", addr.ID, "user_id", authUserID)
	return c.Status(fiber.StatusCreated).JSON(dto.ToAddressResponse(addr))
}

func (h *AddressHandler) ListAddresses(c *fiber.Ctx) error {
	ctx := c.UserContext()
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		slog.WarnContext(ctx, "missing authenticated user context in HTTP ListAddresses")
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	addresses, err := h.addressService.ListAddresses(c.Context(), authUserID)
	if err != nil {
		slog.WarnContext(ctx, "service error in HTTP ListAddresses", "error", err)
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	slog.InfoContext(ctx, "HTTP ListAddresses succeeded", "user_id", authUserID, "count", len(addresses))
	return c.JSON(dto.ToAddressListResponse(addresses))
}

func (h *AddressHandler) GetAddress(c *fiber.Ctx) error {
	ctx := c.UserContext()
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		slog.WarnContext(ctx, "missing authenticated user context in HTTP GetAddress")
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	addressID := c.Params("id")
	addr, err := h.addressService.GetAddress(c.Context(), authUserID, addressID)
	if err != nil {
		slog.WarnContext(ctx, "service error in HTTP GetAddress", "address_id", addressID, "error", err)
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	slog.InfoContext(ctx, "HTTP GetAddress succeeded", "address_id", addr.ID, "user_id", authUserID)
	return c.JSON(dto.ToAddressResponse(addr))
}

func (h *AddressHandler) UpdateAddress(c *fiber.Ctx) error {
	ctx := c.UserContext()
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		slog.WarnContext(ctx, "missing authenticated user context in HTTP UpdateAddress")
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	addressID := c.Params("id")
	var req dto.UpdateAddressRequest
	if err := c.BodyParser(&req); err != nil {
		slog.WarnContext(ctx, "invalid request payload in HTTP UpdateAddress", "error", err)
		appErr := errors.BadRequest("invalid request payload")
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	addr, err := h.addressService.UpdateAddress(c.Context(), authUserID, addressID, req)
	if err != nil {
		slog.WarnContext(ctx, "service error in HTTP UpdateAddress", "address_id", addressID, "error", err)
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	slog.InfoContext(ctx, "HTTP UpdateAddress succeeded", "address_id", addr.ID, "user_id", authUserID)
	return c.JSON(dto.ToAddressResponse(addr))
}

func (h *AddressHandler) DeleteAddress(c *fiber.Ctx) error {
	ctx := c.UserContext()
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		slog.WarnContext(ctx, "missing authenticated user context in HTTP DeleteAddress")
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	addressID := c.Params("id")
	if err := h.addressService.DeleteAddress(c.Context(), authUserID, addressID); err != nil {
		slog.WarnContext(ctx, "service error in HTTP DeleteAddress", "address_id", addressID, "error", err)
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	slog.InfoContext(ctx, "HTTP DeleteAddress succeeded", "address_id", addressID, "user_id", authUserID)
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *AddressHandler) SetDefaultAddress(c *fiber.Ctx) error {
	ctx := c.UserContext()
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		slog.WarnContext(ctx, "missing authenticated user context in HTTP SetDefaultAddress")
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	addressID := c.Params("id")
	addr, err := h.addressService.SetDefaultAddress(c.Context(), authUserID, addressID)
	if err != nil {
		slog.WarnContext(ctx, "service error in HTTP SetDefaultAddress", "address_id", addressID, "error", err)
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	slog.InfoContext(ctx, "HTTP SetDefaultAddress succeeded", "address_id", addr.ID, "user_id", authUserID)
	return c.JSON(dto.ToAddressResponse(addr))
}
