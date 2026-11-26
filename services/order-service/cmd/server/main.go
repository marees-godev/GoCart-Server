package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	inventorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/pkg/health"
	"github.com/marees-godev/GoCart-Server/pkg/kafka"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/metrics"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/service"
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

	// 5. Initialize Kafka producer and outbox publisher if enabled
	if strings.EqualFold(cfg.App.Environment, "production") && isLocalhostBrokers(cfg.Kafka.Brokers) {
		log.Warn("Kafka disabled: in production environment, KAFKA_BROKERS is pointing to localhost:9092 where no broker exists. To enable, configure KAFKA_BROKERS with a valid remote broker address.")
		cfg.Kafka.Enabled = false
		cfg.Outbox.Enabled = false
	}

	// 5b. Probe broker connectivity before starting Kafka producer and outbox publisher
	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		probeCtx, probeCancel := context.WithTimeout(ctx, 2*time.Second)
		healthRes := kafka.CheckHealth(probeCtx, kafka.Config{
			Brokers:        cfg.Kafka.Brokers,
			ConnectTimeout: 2 * time.Second,
		})
		probeCancel()

		if healthRes.Status != "UP" {
			log.Warn("Kafka broker is currently unreachable at startup; background publisher and consumer reconnect logic will retry until broker recovers",
				"brokers", cfg.Kafka.Brokers,
				"error", healthRes.Error,
			)
		}
	}

	outboxStore := outbox.NewStore()
	var producer *kafka.Producer
	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		producer = kafka.NewProducer(kafka.Config{
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

		// 6. Start the outbox publisher as a background goroutine.
		if cfg.Outbox.Enabled {
			outboxPublisher := outbox.NewPublisher(db.Pool, producer, outboxStore, outbox.Config{
				PollInterval: cfg.Outbox.PollInterval,
				BatchSize:    cfg.Outbox.BatchSize,
				MaxRetries:   cfg.Outbox.MaxRetries,
			})
			go outboxPublisher.Start(ctx)
		}
	} else {
		log.Info("Kafka or Outbox publisher disabled via configuration; order events will remain safely in the outbox table",
			"kafka_enabled", cfg.Kafka.Enabled,
			"outbox_enabled", cfg.Outbox.Enabled,
		)
	}

	// 7. Initialize downstream service clients
	var cartClient client.CartClient
	cartConn, cartErr := grpc.NewClient(cfg.GRPC.CartServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if cartErr == nil {
		cartClient = client.NewCartClient(cartpb.NewCartServiceClient(cartConn))
		defer cartConn.Close()
	} else {
		log.Warn("Failed to dial Cart Service", "error", cartErr)
	}

	var prodClient client.ProductClient
	productConn, prodErr := grpc.NewClient(cfg.GRPC.ProductServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if prodErr == nil {
		prodClient = client.NewProductClient(productpb.NewProductServiceClient(productConn))
		defer productConn.Close()
	} else {
		log.Warn("Failed to dial Product Service", "error", prodErr)
	}

	var invClient client.InventoryClient
	inventoryConn, invErr := grpc.NewClient(cfg.GRPC.InventoryServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if invErr == nil {
		invClient = client.NewInventoryClient(inventorypb.NewInventoryServiceClient(inventoryConn))
		defer inventoryConn.Close()
	} else {
		log.Warn("Failed to dial Inventory Service", "error", invErr)
	}

	var deliveryClient client.DeliveryClient
	deliveryConn, delErr := grpc.NewClient(cfg.GRPC.DeliveryServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if delErr == nil {
		deliveryClient = client.NewDeliveryClient(deliverypb.NewDeliveryServiceClient(deliveryConn))
		defer deliveryConn.Close()
	} else {
		log.Warn("Failed to dial Delivery Service", "error", delErr)
	}

	// 8. Initialize business layers
	orderRepo := repository.NewOrderRepo(db, outboxStore)
	orderService := service.NewOrderService(orderRepo, cartClient, prodClient, invClient, deliveryClient, cfg.GRPC.TimeoutSeconds, log)
	orderGRPCHandler := handler.NewOrderGRPCHandler(orderService, log)

	// 8b. Start Kafka event consumer for order lifecycle events (PaymentSuccessful, PaymentFailed)
	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 && producer != nil {
		consumerCfg := kafka.ConsumerConfig{
			GroupID: "order-service-group",
			Topics: []string{
				events.TopicPaymentSuccessful,
				events.TopicPaymentFailed,
				events.TopicOrderShipped,
				events.TopicOrderDelivered,
				events.TopicDeliveryStatusChanged,
				events.TopicDeliveryStatusUpdated,
				events.TopicOrderCancelled,
			},
		}
		orderKafkaConsumer := kafka.NewConsumer(kafka.Config{
			Brokers:        cfg.Kafka.Brokers,
			ClientID:       cfg.Kafka.ClientID,
			ConnectTimeout: 10 * time.Second,
			MaxRetries:     cfg.Outbox.MaxRetries,
			RetryInterval:  500 * time.Millisecond,
			DLQSuffix:      ".dlq",
			RetrySuffix:    ".retry",
		}, consumerCfg, producer)

		orderEvtConsumer := consumer.NewOrderEventConsumer(orderService)
		go func() {
			log.Info("Starting Order Kafka Event Consumer...")
			if err := orderKafkaConsumer.Start(ctx, orderEvtConsumer.HandleEvent); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("Order Kafka event consumer failed", "error", err)
			}
		}()
		defer func() {
			if err := orderKafkaConsumer.Close(); err != nil {
				log.Error("Failed to close Order Kafka consumer", "error", err)
			}
		}()
	}

	// 9. Setup gRPC Server
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcclient.UnaryServerInterceptor(),
		),
	)
	orderpb.RegisterOrderServiceServer(grpcServer, orderGRPCHandler)
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

	// 10. Setup Fiber HTTP server with observability middleware
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

	// Admin outbox endpoints (mount under /admin — gate with auth middleware in production)
	adminOutboxHandler := handler.NewAdminOutboxHandler(outboxStore, db.Pool)
	adminOutboxHandler.Register(app)

	go func() {
		log.Info("HTTP server listening", "service", cfg.App.Name, "port", cfg.HTTP.Port)
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

func isLocalhostBrokers(brokers []string) bool {
	for _, b := range brokers {
		trimmed := strings.TrimSpace(b)
		host, _, err := net.SplitHostPort(trimmed)
		if err != nil {
			host = trimmed
		}
		if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]" {
			return true
		}
	}
	return false
}
