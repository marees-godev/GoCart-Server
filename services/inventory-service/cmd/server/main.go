package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	inventorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	storepb "github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/pkg/health"
	"github.com/marees-godev/GoCart-Server/pkg/kafka"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/metrics"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/consumer"
	inventoryGRPC "github.com/marees-godev/GoCart-Server/services/inventory-service/internal/grpc"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/inventory-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
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

	// 5. Initialize Downstream Clients & Domain Layer
	prodGRPCClient, prodConn, err := grpcclient.NewProductClient(cfg.Services.ProductServiceAddr, 5*time.Second)
	if err != nil {
		log.Error("Failed to initialize product service gRPC client", "error", err)
		os.Exit(1)
	}
	if prodConn != nil {
		defer func() { _ = prodConn.Close() }()
	}

	var storeGRPCClient storepb.StoreServiceClient
	storeCli, storeConn, err := grpcclient.NewStoreClient(cfg.Services.StoreServiceAddr, 5*time.Second)
	if err != nil {
		log.Warn("Failed to initialize store service gRPC client", "error", err)
	} else if storeCli != nil {
		storeGRPCClient = storeCli
		if storeConn != nil {
			defer func() { _ = storeConn.Close() }()
		}
	}

	productClient := client.NewGRPCProductClient(prodGRPCClient, storeGRPCClient)

	invRepo := repository.NewInventoryRepository(db.Pool)
	invService := service.NewInventoryService(invRepo, productClient)
	cleanupInterval := time.Duration(cfg.Reservation.ExpirationCleanupIntervalSeconds) * time.Second
	invService.StartExpirationWorker(ctx, cleanupInterval)
	invGRPCServer := inventoryGRPC.NewInventoryGRPCServer(invService)

	// 6. Initialize Kafka Producer, Outbox Publisher & Event Consumer
	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		kafkaCfg := kafka.Config{
			Brokers:       cfg.Kafka.Brokers,
			MaxRetries:    3,
			RetryInterval: 1 * time.Second,
		}
		kp := kafka.NewProducer(kafkaCfg)
		defer func() { _ = kp.Close() }()

		// Start Outbox Publisher worker
		outboxStore := outbox.NewStore()
		outboxPublisher := outbox.NewPublisher(db.Pool, kp, outboxStore, outbox.DefaultConfig())
		go outboxPublisher.Start(ctx)

		// Start Kafka Event Consumer
		consumerCfg := kafka.ConsumerConfig{
			GroupID: "inventory-service-group",
			Topics: []string{
				events.TopicOrderCreated,
				events.TopicPaymentFailed,
				events.TopicOrderCancelled,
				events.TopicOrderConfirmed,
			},
		}
		kafkaConsumer := kafka.NewConsumer(kafkaCfg, consumerCfg, kp)
		evtConsumer := consumer.NewInventoryEventConsumer(invService)
		go func() {
			log.Info("Starting Inventory Kafka Event Consumer...")
			if err := kafkaConsumer.Start(ctx, evtConsumer.HandleEvent); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("Kafka event consumer failed", "error", err)
			}
		}()
		defer func() { _ = kafkaConsumer.Close() }()
	}

	// 6. Start gRPC Server
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpcclient.UnaryServerInterceptor()),
		grpc.MaxRecvMsgSize(10*1024*1024),
		grpc.MaxSendMsgSize(10*1024*1024),
	)
	inventorypb.RegisterInventoryServiceServer(grpcServer, invGRPCServer)
	reflection.Register(grpcServer)

	grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.GRPC.Port))
	if err != nil {
		log.Error("Failed to listen for gRPC", "port", cfg.GRPC.Port, "error", err)
		os.Exit(1)
	}

	go func() {
		log.Info("gRPC server listening", "service", cfg.App.Name, "port", cfg.GRPC.Port)
		if err := grpcServer.Serve(grpcLis); err != nil {
			log.Error("gRPC server failed", "error", err)
		}
	}()

	// 7. Setup Fiber HTTP server with observability middleware
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

	go func() {
		log.Info("HTTP service listening", "service", cfg.App.Name, "port", cfg.HTTP.Port)
		if err := app.Listen(fmt.Sprintf(":%s", cfg.HTTP.Port)); err != nil {
			log.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	cancel()
	log.Info("Shutting down service gracefully", "service", cfg.App.Name)

	grpcServer.GracefulStop()

	if err := app.Shutdown(); err != nil {
		log.Error("Failed to gracefully shutdown HTTP server", "error", err)
	}

	log.Info("Service stopped", "service", cfg.App.Name)
}
