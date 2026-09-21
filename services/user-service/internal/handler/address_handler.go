package handler

import (
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
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	var req dto.CreateAddressRequest
	if err := c.BodyParser(&req); err != nil {
		appErr := errors.BadRequest("invalid request payload")
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	addr, err := h.addressService.CreateAddress(c.Context(), authUserID, req)
	if err != nil {
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	return c.Status(fiber.StatusCreated).JSON(dto.ToAddressResponse(addr))
}

func (h *AddressHandler) ListAddresses(c *fiber.Ctx) error {
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	addresses, err := h.addressService.ListAddresses(c.Context(), authUserID)
	if err != nil {
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	return c.JSON(dto.ToAddressListResponse(addresses))
}

func (h *AddressHandler) GetAddress(c *fiber.Ctx) error {
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	addressID := c.Params("id")
	addr, err := h.addressService.GetAddress(c.Context(), authUserID, addressID)
	if err != nil {
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	return c.JSON(dto.ToAddressResponse(addr))
}

func (h *AddressHandler) UpdateAddress(c *fiber.Ctx) error {
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	addressID := c.Params("id")
	var req dto.UpdateAddressRequest
	if err := c.BodyParser(&req); err != nil {
		appErr := errors.BadRequest("invalid request payload")
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	addr, err := h.addressService.UpdateAddress(c.Context(), authUserID, addressID, req)
	if err != nil {
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	return c.JSON(dto.ToAddressResponse(addr))
}

func (h *AddressHandler) DeleteAddress(c *fiber.Ctx) error {
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	addressID := c.Params("id")
	if err := h.addressService.DeleteAddress(c.Context(), authUserID, addressID); err != nil {
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *AddressHandler) SetDefaultAddress(c *fiber.Ctx) error {
	authUserID := getAuthUserID(c)
	if authUserID == "" {
		err := errors.Unauthorized("missing authenticated user context")
		return c.Status(err.HTTPStatus).JSON(err.ToResponse())
	}

	addressID := c.Params("id")
	addr, err := h.addressService.SetDefaultAddress(c.Context(), authUserID, addressID)
	if err != nil {
		appErr := errors.AsAppError(err)
		return c.Status(appErr.HTTPStatus).JSON(appErr.ToResponse())
	}

	return c.JSON(dto.ToAddressResponse(addr))
}
