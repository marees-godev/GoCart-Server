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
	"github.com/marees-godev/GoCart-Server/contracts/events"
	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/pkg/health"
	"github.com/marees-godev/GoCart-Server/pkg/kafka"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/metrics"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/consumer"
	deliveryGRPC "github.com/marees-godev/GoCart-Server/services/delivery-service/internal/grpc"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/service"
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

	// 5. Initialize Kafka producer
	producer := kafka.NewProducer(kafka.Config{
		Brokers:        cfg.Kafka.Brokers,
		ClientID:       cfg.Kafka.ClientID,
		ConnectTimeout: 10 * time.Second,
		MaxRetries:     cfg.Outbox.MaxRetries,
		RetryInterval:  500 * time.Millisecond,
	})
	defer func() {
		if err := producer.Close(); err != nil {
			log.Error("Failed to close Kafka producer", "error", err)
		}
	}()

	// 6. Initialize outbox store, repository & service
	outboxStore := outbox.NewStore()
	deliveryRepo := repository.NewDeliveryRepositoryWithOutbox(db, outboxStore, log)
	deliveryService := service.NewDeliveryService(deliveryRepo, log)

	// 7. Start transactional outbox publisher as a background worker
	if cfg.Outbox.Enabled {
		outboxPublisher := outbox.NewPublisher(db.Pool, producer, outboxStore, outbox.Config{
			ServiceName:    cfg.App.Name,
			PollInterval:   cfg.Outbox.PollInterval,
			BatchSize:      cfg.Outbox.BatchSize,
			MaxRetries:     cfg.Outbox.MaxRetries,
			BaseRetryDelay: cfg.Outbox.BaseRetryDelay,
			MaxRetryDelay:  cfg.Outbox.MaxRetryDelay,
			DLQTopic:       cfg.Outbox.DLQTopic,
		})
		go outboxPublisher.Start(ctx)
	} else {
		log.Info("Outbox publisher is disabled via OUTBOX_ENABLED=false")
	}

	// 8. Setup Kafka event consumer
	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		consumerCfg := kafka.ConsumerConfig{
			GroupID: "delivery-service-group",
			Topics: []string{
				events.TopicOrderConfirmed,
				events.TopicPartnerRegistered,
			},
		}
		kafkaConsumer := kafka.NewConsumer(kafka.Config{
			Brokers:        cfg.Kafka.Brokers,
			ClientID:       cfg.Kafka.ClientID,
			ConnectTimeout: 10 * time.Second,
		}, consumerCfg, producer)
		evtConsumer := consumer.NewDeliveryEventConsumer(deliveryService)
		go func() {
			log.Info("Starting Delivery Kafka Event Consumer...")
			if err := kafkaConsumer.Start(ctx, evtConsumer.HandleEvent); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("Kafka event consumer failed", "error", err)
			}
		}()
		defer func() { _ = kafkaConsumer.Close() }()
	}

	// 9. Setup Fiber HTTP server with observability middleware
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(adaptor.HTTPMiddleware(middleware.Recovery))
	app.Use(adaptor.HTTPMiddleware(middleware.RequestID))
	app.Use(adaptor.HTTPMiddleware(middleware.Tracing(cfg.App.Name)))
	app.Use(adaptor.HTTPMiddleware(middleware.Metrics(cfg.App.Name)))
	app.Use(adaptor.HTTPMiddleware(middleware.Logger))

	// Health and Metrics endpoints
	healthHandler := health.NewHandler(cfg.App.Name, health.FromPinger(db))
	healthHandler.Register(app)
	app.Get("/metrics", adaptor.HTTPHandler(metrics.Handler()))

	// Tracking and Partner HTTP routes
	deliveryHandler := handler.NewDeliveryHandler(deliveryService, cfg.JWT.Secret)
	deliveryHandler.RegisterRoutes(app)

	// 10. Setup gRPC server
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpcclient.UnaryServerInterceptor()),
		grpc.MaxRecvMsgSize(10*1024*1024),
		grpc.MaxSendMsgSize(10*1024*1024),
	)
	deliveryGRPCServer := deliveryGRPC.NewDeliveryGRPCServer(deliveryService, log)
	deliverypb.RegisterDeliveryServiceServer(grpcServer, deliveryGRPCServer)
	reflection.Register(grpcServer)

	grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.GRPC.Port))
	if err != nil {
		log.Error("Failed to listen for gRPC", "port", cfg.GRPC.Port, "error", err)
		os.Exit(1)
	}

	go func() {
		log.Info("gRPC server listening", "service", cfg.App.Name, "port", cfg.GRPC.Port)
		if err := grpcServer.Serve(grpcLis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			log.Error("gRPC server failed", "error", err)
		}
	}()

	go func() {
		log.Info("HTTP service listening", "service", cfg.App.Name, "port", cfg.HTTP.Port)
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
