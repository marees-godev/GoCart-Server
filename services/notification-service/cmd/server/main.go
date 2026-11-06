package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/health"
	"github.com/marees-godev/GoCart-Server/pkg/kafka"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/metrics"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
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

	// 5. Initialize Repositories and Services
	repo := repository.NewNotificationRepository(db.Pool)
	tmplSvc := service.NewTemplateService(repo)
	prefSvc := service.NewPreferenceService(repo, repo)
	pipelineSvc := service.NewNotificationPipelineService(tmplSvc, prefSvc, repo, repo, log)
	eventConsumer := consumer.NewNotificationEventConsumer(pipelineSvc, log)

	// 6. Start Asynchronous Notification Job Worker
	jobWorker := service.NewJobWorker(repo, log, 5*time.Second)
	go jobWorker.Start(ctx)

	// 7. Initialize Kafka Consumer
	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		producer := kafka.NewProducer(kafka.Config{
			Brokers:  cfg.Kafka.Brokers,
			ClientID: cfg.Kafka.ClientID,
		})
		defer producer.Close()

		topics := []string{
			events.TopicUserRegistered,
			events.TopicStoreApproved,
			events.TopicStoreRejected,
			events.TopicOrderCreated,
			events.TopicPaymentSuccessful,
			events.TopicPaymentFailed,
			events.TopicOrderShipped,
			events.TopicOrderDelivered,
			events.TopicReturnApproved,
			events.TopicReturnRejected,
			events.TopicRefundCompleted,
		}

		kafkaConsumer := kafka.NewConsumer(
			kafka.Config{
				Brokers:        cfg.Kafka.Brokers,
				ClientID:       cfg.Kafka.ClientID,
				ConnectTimeout: 10 * time.Second,
			},
			kafka.ConsumerConfig{
				GroupID: cfg.Kafka.GroupID,
				Topics:  topics,
			},
			producer,
		)
		defer kafkaConsumer.Close()

		go func() {
			log.Info("Starting Notification Kafka Event Consumer...", "topics", topics, "group", cfg.Kafka.GroupID)
			if err := kafkaConsumer.Start(ctx, eventConsumer.HandleEvent); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("Kafka event consumer failed", "error", err)
			}
		}()
	}

	// 8. Setup Fiber HTTP server with observability middleware
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(adaptor.HTTPMiddleware(middleware.Recovery))
	app.Use(adaptor.HTTPMiddleware(middleware.RequestID))
	app.Use(adaptor.HTTPMiddleware(middleware.Tracing(cfg.App.Name)))
	app.Use(adaptor.HTTPMiddleware(middleware.Metrics(cfg.App.Name)))
	app.Use(adaptor.HTTPMiddleware(middleware.Logger))

	// Observability endpoints
	healthHandler := health.NewHandler(cfg.App.Name, health.FromPinger(db))
	healthHandler.Register(app)
	app.Get("/metrics", adaptor.HTTPHandler(metrics.Handler()))

	go func() {
		log.Info("Service listening", "service", cfg.App.Name, "port", cfg.HTTP.Port)
		if err := app.Listen(fmt.Sprintf(":%s", cfg.HTTP.Port)); err != nil {
			log.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("Shutting down service gracefully", "service", cfg.App.Name)

	if err := app.Shutdown(); err != nil {
		log.Error("Failed to gracefully shutdown HTTP server", "error", err)
	}

	log.Info("Service stopped", "service", cfg.App.Name)
}
