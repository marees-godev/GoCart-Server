package router_test

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/router"
	"github.com/stretchr/testify/assert"
)

func TestSetupRoutes(t *testing.T) {
	app := fiber.New()
	userHandler := handler.NewUserHandler(nil)
	addressHandler := handler.NewAddressHandler(nil)

	router.SetupRoutes(app, userHandler, addressHandler)

	routes := app.GetRoutes()
	registeredPaths := make(map[string]bool)
	for _, r := range routes {
		registeredPaths[r.Method+":"+r.Path] = true
	}

	assert.True(t, registeredPaths["GET:/api/v1/users/me"])
	assert.True(t, registeredPaths["GET:/api/v1/users/:id"])
	assert.True(t, registeredPaths["PUT:/api/v1/users/:id"])
	assert.True(t, registeredPaths["POST:/api/v1/users/addresses"])
	assert.True(t, registeredPaths["GET:/api/v1/users/addresses"])
	assert.True(t, registeredPaths["GET:/api/v1/users/addresses/:id"])
	assert.True(t, registeredPaths["PUT:/api/v1/users/addresses/:id"])
	assert.True(t, registeredPaths["DELETE:/api/v1/users/addresses/:id"])
	assert.True(t, registeredPaths["PUT:/api/v1/users/addresses/:id/default"])
}
