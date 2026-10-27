package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	returnpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/return"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/pkg/health"
	"github.com/marees-godev/GoCart-Server/pkg/kafka"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/metrics"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
		log.Error("Failed to initialize distributed tracing", slog.Any("error", err))
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
		log.Error("Failed to connect to database", slog.Any("error", err))
		os.Exit(1)
	}
	defer db.Close()

	// 4. Run auto migrations if enabled
	if cfg.Database.AutoMigrate {
		migrator, err := database.NewMigrator(db.Pool, cfg.Database.MigrationsPath)
		if err != nil {
			log.Error("Failed to initialize migrator", slog.Any("error", err))
			os.Exit(1)
		}
		if err := migrator.Run(ctx); err != nil {
			log.Error("Failed to execute migrations", slog.Any("error", err))
			os.Exit(1)
		}
	}

	// 5. Initialize Kafka producer and Outbox publisher if enabled
	outboxStore := outbox.NewStore()
	if cfg.Kafka.Enabled && cfg.Outbox.Enabled && len(cfg.Kafka.Brokers) > 0 {
		producer := kafka.NewProducer(kafka.Config{
			Brokers:        cfg.Kafka.Brokers,
			ClientID:       cfg.Kafka.ClientID,
			ConnectTimeout: 10 * time.Second,
			MaxRetries:     cfg.Outbox.MaxRetries,
			RetryInterval:  500 * time.Millisecond,
		})
		defer func() {
			if err := producer.Close(); err != nil {
				log.Error("Failed to close Kafka producer", slog.Any("error", err))
			}
		}()

		outboxPublisher := outbox.NewPublisher(db.Pool, producer, outboxStore, outbox.Config{
			ServiceName:  cfg.App.Name,
			PollInterval: cfg.Outbox.PollInterval,
			BatchSize:    cfg.Outbox.BatchSize,
			MaxRetries:   cfg.Outbox.MaxRetries,
		})
		go outboxPublisher.Start(ctx)
		log.Info("Kafka outbox publisher started successfully",
			slog.String("client_id", cfg.Kafka.ClientID),
			slog.Any("brokers", cfg.Kafka.Brokers),
		)
	} else {
		log.Info("Kafka producer or Outbox publisher disabled via configuration; return events will remain safely persisted in the outbox table",
			slog.Bool("kafka_enabled", cfg.Kafka.Enabled),
			slog.Bool("outbox_enabled", cfg.Outbox.Enabled),
		)
	}

	// 6. Initialize downstream service clients
	var orderClient client.OrderClient
	orderConn, orderErr := grpc.NewClient(cfg.GRPC.OrderServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if orderErr == nil {
		orderClient = client.NewOrderClient(orderpb.NewOrderServiceClient(orderConn))
		defer orderConn.Close()
	} else {
		log.Warn("Failed to dial Order Service", slog.Any("error", orderErr))
	}

	var productClient client.ProductClient
	productConn, prodErr := grpc.NewClient(cfg.GRPC.ProductServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if prodErr == nil {
		productClient = client.NewProductClient(productpb.NewProductServiceClient(productConn))
		defer productConn.Close()
	} else {
		log.Warn("Failed to dial Product Service", slog.Any("error", prodErr))
	}

	// 7. Initialize business layers
	returnRepo := repository.NewReturnRepository(db, outboxStore, log)
	returnService := service.NewReturnService(returnRepo, orderClient, productClient, cfg.Return.ReturnWindowDays, log)
	returnGRPCHandler := handler.NewReturnGRPCHandler(returnService, log)

	// 8. Setup gRPC Server
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcclient.UnaryServerInterceptor(),
		),
	)
	returnpb.RegisterReturnServiceServer(grpcServer, returnGRPCHandler)
	reflection.Register(grpcServer)

	grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.GRPC.Port))
	if err != nil {
		log.Error("Failed to listen for gRPC", slog.String("port", cfg.GRPC.Port), slog.Any("error", err))
		os.Exit(1)
	}

	go func() {
		log.Info("gRPC server listening", slog.String("service", cfg.App.Name), slog.String("port", cfg.GRPC.Port))
		if err := grpcServer.Serve(grpcLis); err != nil {
			log.Error("gRPC server failed", slog.Any("error", err))
		}
	}()

	// 9. Setup Fiber HTTP server with observability middleware (metrics, health)
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
		log.Info("HTTP server listening", slog.String("service", cfg.App.Name), slog.String("port", cfg.HTTP.Port))
		if err := app.Listen(fmt.Sprintf(":%s", cfg.HTTP.Port)); err != nil {
			log.Error("HTTP server failed", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("Shutting down service gracefully", slog.String("service", cfg.App.Name))

	grpcServer.GracefulStop()

	if err := app.Shutdown(); err != nil {
		log.Error("Failed to gracefully shutdown HTTP server", slog.Any("error", err))
	}

	log.Info("Service stopped", slog.String("service", cfg.App.Name))
}
