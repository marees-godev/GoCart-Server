package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	gwGraphQL "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql"
	gwResolver "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolver"
	"github.com/marees-godev/GoCart-Server/pkg/health"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/metrics"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
)

func main() {
	cfg := config.LoadEnv()

	// 1. Initialize structured logger
	log := logger.New(logger.Config{
		ServiceName: cfg.App.Name,
		Environment: cfg.App.Environment,
		Version:     cfg.App.Version,
		Level:       cfg.Logger.Level,
		Format:      cfg.Logger.Format,
	})

	log.Info("Starting API Gateway", "service", cfg.App.Name, "version", cfg.App.Version, "env", cfg.App.Environment)

	// 2. Initialize distributed tracing if enabled
	tp, err := tracing.Init(tracing.Config{
		ServiceName: cfg.App.Name,
		Environment: cfg.App.Environment,
		Version:     cfg.App.Version,
		Enabled:     cfg.Tracing.Enabled,
		Exporter:    cfg.Tracing.Exporter,
	})
	if err != nil {
		log.Error("Failed to initialize distributed tracing", "error", err)
		os.Exit(1)
	}
	if tp != nil {
		defer func() { _ = tp.Shutdown(context.Background()) }()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 3. Setup Fiber HTTP server with observability middleware
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(adaptor.HTTPMiddleware(middleware.Recovery))
	app.Use(adaptor.HTTPMiddleware(middleware.RequestID))
	app.Use(adaptor.HTTPMiddleware(middleware.Tracing(cfg.App.Name)))
	app.Use(adaptor.HTTPMiddleware(middleware.Metrics(cfg.App.Name)))
	app.Use(adaptor.HTTPMiddleware(middleware.Logger))

	// 4. Register Health and Readiness endpoints
	healthHandler := health.NewHandler(cfg.App.Name)
	healthHandler.Register(app)
	app.Get("/metrics", adaptor.HTTPHandler(metrics.Handler()))

	// 5. Initialize and register GraphQL server
	gqlResolver := gwResolver.NewResolver(cfg.App.Version)
	gqlServer := gwGraphQL.NewServer(gqlResolver)

	app.All("/graphql", adaptor.HTTPHandler(gqlServer))

	if cfg.GraphQL.PlaygroundEnabled {
		playgroundHandler := gwGraphQL.PlaygroundHandler("GoCart API Gateway GraphQL", "/graphql")
		app.Get("/playground", adaptor.HTTPHandler(playgroundHandler))
		app.Get("/", adaptor.HTTPHandler(playgroundHandler))
	}

	go func() {
		log.Info("API Gateway listening", "service", cfg.App.Name, "port", cfg.HTTP.Port)
		if err := app.Listen(fmt.Sprintf(":%s", cfg.HTTP.Port)); err != nil {
			log.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("Shutting down API Gateway gracefully", "service", cfg.App.Name)

	if err := app.Shutdown(); err != nil {
		log.Error("Failed to gracefully shutdown HTTP server", "error", err)
	}

	log.Info("API Gateway stopped", "service", cfg.App.Name)
}
