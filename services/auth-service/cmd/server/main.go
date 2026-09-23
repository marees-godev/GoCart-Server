package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	pb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/pkg/health"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/metrics"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
	authGRPC "github.com/marees-godev/GoCart-Server/services/auth-service/internal/handler/grpc"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/service"
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

	// 5. Setup Fiber HTTP server with observability middleware
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

	// 6. Initialize business logic layers & gRPC handler
	userClient, userConn, err := grpcclient.NewUserClient(cfg.UserServiceAddr, 5*time.Second)
	if err != nil {
		log.Error("Failed to initialize required user service client", "addr", cfg.UserServiceAddr, "error", err)
		os.Exit(1)
	}
	defer userConn.Close()

	var merchantClient merchantpb.MerchantServiceClient
	if cfg.Services.MerchantServiceURL != "" {
		mClient, conn, err := grpcclient.NewMerchantClient(cfg.Services.MerchantServiceURL, 5*time.Second)
		if err != nil {
			log.Warn("Failed to initialize merchant service gRPC client", "error", err)
		} else {
			defer conn.Close()
			merchantClient = mClient
		}
	}

	authRepo := repository.NewAuthRepository(db.Pool, log)
	authSvc := service.NewAuthService(authRepo, cfg, log, userClient, merchantClient)

	// 7. Initialize gRPC server for all Auth operations
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(grpcclient.UnaryServerInterceptor()))
	grpcHandler := authGRPC.NewAuthGRPCHandler(authSvc, log)
	pb.RegisterAuthServiceServer(grpcServer, grpcHandler)

	serverErr := make(chan error, 2)

	go func() {
		log.Info("HTTP service listening", "service", cfg.App.Name, "port", cfg.HTTP.Port)
		if err := app.Listen(fmt.Sprintf(":%s", cfg.HTTP.Port)); err != nil {
			serverErr <- err
			return
		}
	}()

	go func() {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.GRPC.Port))
		if err != nil {
			log.Error("Failed to listen for gRPC", "error", err)
			serverErr <- err
			return
		}
		log.Info("gRPC service listening", "service", cfg.App.Name, "port", cfg.GRPC.Port)
		if err := grpcServer.Serve(lis); err != nil {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		log.Error("Server failed", "error", err)
	case <-ctx.Done():
		log.Info("Shutting down service gracefully", "service", cfg.App.Name)
	}

	grpcServer.GracefulStop()
	if err := app.Shutdown(); err != nil {
		log.Error("Failed to gracefully shutdown HTTP server", "error", err)
	}

	log.Info("Service stopped", "service", cfg.App.Name)
}

