package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/handler"
)

func SetupRoutes(app fiber.Router, userHandler *handler.UserHandler) {
	api := app.Group("/api/v1")

	users := api.Group("/users")
	users.Get("/me", userHandler.GetMyProfile)
	users.Get("/:id", userHandler.GetUserByID)
	users.Put("/:id", userHandler.UpdateProfile)
}
