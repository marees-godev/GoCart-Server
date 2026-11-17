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
	personalizationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/personalization"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/pkg/health"
	"github.com/marees-godev/GoCart-Server/pkg/kafka"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/metrics"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/service"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.Load()

	// 1. Logger
	log := logger.New(logger.Config{
		ServiceName: cfg.App.Name,
		Environment: cfg.App.Environment,
		Version:     cfg.App.Version,
		Level:       cfg.Logger.Level,
		Format:      cfg.Logger.Format,
	})

	// 2. Distributed Tracing
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

	// 3. Database Connection Pool
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

	// 3. Run auto migrations if enabled
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

	// 4. Repository
	repo := repository.NewActivityRepository(db.Pool)

	// 5. Kafka Producer & Consumer
	var producer *kafka.Producer
	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		producer = kafka.NewProducer(kafka.Config{
			Brokers:  cfg.Kafka.Brokers,
			ClientID: cfg.Kafka.ClientID,
		})
		defer producer.Close()
	}

	// 6. Service & Consumer
	svc := service.NewPersonalizationService(repo, producer, log)
	evtConsumer := consumer.NewActivityConsumer(svc, log)

	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 && producer != nil {
		topics := []string{
			events.TopicCustomerActivity,
		}

		kafkaConsumer := kafka.NewConsumer(
			kafka.Config{
				Brokers:        cfg.Kafka.Brokers,
				ClientID:       cfg.Kafka.ClientID,
				ConnectTimeout: 10 * time.Second,
				MaxRetries:     3,
				RetryInterval:  1 * time.Second,
				DLQSuffix:      ".dlq",
			},
			kafka.ConsumerConfig{
				GroupID: cfg.Kafka.GroupID,
				Topics:  topics,
			},
			producer,
		)
		defer kafkaConsumer.Close()

		// Runs continously in the background
		go func() {
			log.Info("Starting Personalization Kafka Event Consumer...", "topics", topics, "group", cfg.Kafka.GroupID)
			if err := kafkaConsumer.Start(ctx, evtConsumer.HandleEvent); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("kafka event consumer failed", "error", err)
			}
		}()
	}

	// 6. gPRC Server
	grpcHandler := handler.NewPersonalizationGRPCHandler(svc, log)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.GRPC.Port))
	if err != nil {
		log.Error("failed to listen for gRPC", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcclient.UnaryServerInterceptor(),
		),
	)
	personalizationpb.RegisterPersonalizationServiceServer(grpcServer, grpcHandler)

	go func() {
		log.Info("gRPC server listening", "service", cfg.App.Name, "port", cfg.GRPC.Port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Error("gRPC server failed", "error", err)
		}
	}()

	// 7. HTTP Server for Health / Observability (/health, /ready, /metrics)
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	app.Use(adaptor.HTTPMiddleware(middleware.Recovery))
	app.Use(adaptor.HTTPMiddleware(middleware.RequestID))
	app.Use(adaptor.HTTPMiddleware(middleware.Tracing(cfg.App.Name)))
	app.Use(adaptor.HTTPMiddleware(middleware.Metrics(cfg.App.Name)))
	app.Use(adaptor.HTTPMiddleware(middleware.Logger))
	healthHandler := health.NewHandler(cfg.App.Name, health.FromPinger(db))
	healthHandler.Register(app)
	app.Get("/metrics", adaptor.HTTPHandler(metrics.Handler()))
	go func() {
		log.Info("HTTP service listening", "service", cfg.App.Name, "port", cfg.HTTP.Port)
		if err := app.Listen(fmt.Sprintf(":%s", cfg.HTTP.Port)); err != nil {
			log.Error("HTTP server failed", "error", err)
		}
	}()

	<-ctx.Done()
	log.Info("Shutting down personalization service gracefully...")

	grpcServer.GracefulStop()
	_ = app.Shutdown()
}
