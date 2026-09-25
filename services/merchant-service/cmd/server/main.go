package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/pkg/health"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/metrics"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/config"
	merchantGRPC "github.com/marees-godev/GoCart-Server/services/merchant-service/internal/grpc"
	merchantMiddleware "github.com/marees-godev/GoCart-Server/services/merchant-service/internal/middleware"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/service"
	"google.golang.org/grpc"
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

	// 2. Initialize distributed tracing
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

	// 3. Initialize database connection pool
	db, err := database.New(ctx, database.Config{
		URL:            cfg.Database.URL,
		MaxConns:       cfg.Database.MaxConns,
		MinConns:       cfg.Database.MinConns,
		ConnectTimeout: 25 * time.Second,
		MaxRetries:     4,
		RetryInterval:  1 * time.Second,
	})
	if err != nil {
		log.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// 4. Run auto migrations if enabled
	if cfg.Database.AutoMigrate {
		migrator, err := database.NewMigrator(db.Pool, cfg.Database.MigrationsPath)
		if err != nil {
			log.Error("Failed to initialize migrator", "error", err)
			os.Exit(1)
		}
		if err := migrator.Run(ctx); err != nil {
			log.Error("Failed to execute migrations", "error", err)
			os.Exit(1)
		}
	}

	// 5. Initialize repository & service
	merchantRepo := repository.NewMerchantRepository(db, log)
	merchantService := service.NewMerchantService(merchantRepo, log)

	// 6. Setup Fiber HTTP server with observability middleware
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(adaptor.HTTPMiddleware(middleware.Recovery))
	app.Use(adaptor.HTTPMiddleware(middleware.RequestID))
	app.Use(adaptor.HTTPMiddleware(middleware.Tracing(cfg.App.Name)))
	app.Use(adaptor.HTTPMiddleware(middleware.Metrics(cfg.App.Name)))
	app.Use(adaptor.HTTPMiddleware(middleware.Logger))

	// Metrics, Health and Readiness endpoints
	healthHandler := health.NewHandler(cfg.App.Name, health.FromPinger(db))
	healthHandler.Register(app)
	app.Get("/metrics", adaptor.HTTPHandler(metrics.Handler()))

	// 8. Setup gRPC server with logging/tracing and ownership middleware
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcclient.UnaryServerInterceptor(),
			merchantMiddleware.UnaryOwnershipInterceptor(merchantRepo, log),
		),
	)
	merchantGRPCServer := merchantGRPC.NewMerchantGRPCServer(merchantService, log)
	merchantpb.RegisterMerchantServiceServer(grpcServer, merchantGRPCServer)

	grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.GRPC.Port))
	if err != nil {
		log.Error("Failed to listen on gRPC port", "port", cfg.GRPC.Port, "error", err)
		os.Exit(1)
	}

	go func() {
		log.Info("gRPC Service listening", "service", cfg.App.Name, "port", cfg.GRPC.Port)
		if err := grpcServer.Serve(grpcLis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			log.Error("gRPC server failed", "error", err)
			os.Exit(1)
		}
	}()

	go func() {
		log.Info("HTTP Service listening", "service", cfg.App.Name, "port", cfg.HTTP.Port)
		if err := app.Listen(fmt.Sprintf(":%s", cfg.HTTP.Port)); err != nil {
			log.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("Shutting down service gracefully", "service", cfg.App.Name)

	grpcServer.GracefulStop()
	if err := app.Shutdown(); err != nil {
		log.Error("Failed to gracefully shutdown HTTP server", "error", err)
	}

	log.Info("Service stopped", "service", cfg.App.Name)
}
