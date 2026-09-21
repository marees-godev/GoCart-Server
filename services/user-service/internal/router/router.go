package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/handler"
)

func SetupRoutes(app fiber.Router, userHandler *handler.UserHandler, addressHandler *handler.AddressHandler) {
	api := app.Group("/api/v1")

	users := api.Group("/users")
	users.Get("/me", userHandler.GetMyProfile)
	users.Get("/:id", userHandler.GetUserByID)
	users.Put("/:id", userHandler.UpdateProfile)

	addresses := users.Group("/addresses")
	addresses.Post("", addressHandler.CreateAddress)
	addresses.Get("", addressHandler.ListAddresses)
	addresses.Get("/:id", addressHandler.GetAddress)
	addresses.Put("/:id", addressHandler.UpdateAddress)
	addresses.Delete("/:id", addressHandler.DeleteAddress)
	addresses.Put("/:id/default", addressHandler.SetDefaultAddress)
}
