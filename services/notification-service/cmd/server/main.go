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
	notificationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/notification"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/pkg/health"
	"github.com/marees-godev/GoCart-Server/pkg/kafka"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/metrics"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/push"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/provider"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

func main() {
	cfg := config.LoadEnv()

	log := logger.New(logger.Config{
		ServiceName: cfg.App.Name,
		Environment: cfg.App.Environment,
		Version:     cfg.App.Version,
		Level:       cfg.Logger.Level,
		Format:      cfg.Logger.Format,
	})

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

	var orderLookup service.OrderUserLookup
	if cfg.GRPC.OrderServiceAddr != "" {
		orderConn, ordErr := grpc.NewClient(
			cfg.GRPC.OrderServiceAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if ordErr == nil {
			defer orderConn.Close()
			orderLookup = service.NewGRPCOrderUserLookup(orderpb.NewOrderServiceClient(orderConn), cfg.GRPC.LookupTimeout)
		} else {
			log.Warn("Failed to dial Order Service for user lookup fallback", "error", ordErr)
		}
	}

	var userLookup service.UserEmailLookup
	if cfg.GRPC.UserServiceAddr != "" {
		userConn, uErr := grpc.NewClient(
			cfg.GRPC.UserServiceAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if uErr == nil {
			defer userConn.Close()
			userLookup = service.NewGRPCUserEmailLookup(userpb.NewUserServiceClient(userConn), cfg.GRPC.LookupTimeout)
		} else {
			log.Warn("Failed to dial User Service for user email lookup", "error", uErr)
		}
	}

	pushClient := push.NewWebPushClient(cfg.VAPID, cfg.Push, nil, log)
	if !pushClient.IsConfigured() {
		log.Warn("VAPID keys are not configured; Web Push delivery will be marked FAILED when attempted")
	}
	localNotifier := push.NewLocalSystemNotifier(log, cfg.Push.LocalNotify)

	repo := repository.NewNotificationRepository(db.Pool)
	tmplSvc := service.NewTemplateService(repo)
	prefSvc := service.NewPreferenceService(repo, repo)
	subSvc := service.NewPushSubscriptionService(repo, cfg.VAPID, log)
	notifSvc := service.NewNotificationService(repo, repo, repo, repo, tmplSvc, pushClient, log, localNotifier)
	pipelineSvc := service.NewPushNotificationPipelineService(tmplSvc, prefSvc, repo, repo, notifSvc, orderLookup, log, userLookup)
	eventConsumer := consumer.NewNotificationEventConsumer(pipelineSvc, log)
	grpcHandler := handler.NewNotificationGRPCHandler(notifSvc, subSvc)

	// 6. Start Asynchronous Notification Job Worker with Provider Registry
	providerRegistry := provider.NewRegistryFromConfig(cfg.Providers, log)
	jobWorker := service.NewJobWorker(repo, providerRegistry, log, 5*time.Second)
	go jobWorker.Start(ctx)

	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		producer := kafka.NewProducer(kafka.Config{
			Brokers:       cfg.Kafka.Brokers,
			ClientID:      cfg.Kafka.ClientID,
			MaxRetries:    cfg.Kafka.MaxRetries,
			RetryInterval: cfg.Kafka.RetryInterval,
			DLQSuffix:     cfg.Kafka.DLQSuffix,
		})
		defer producer.Close()

		eventConsumer.SetDLQPublisher(func(pCtx context.Context, envelope *events.EventEnvelope, reason error) error {
			if envelope == nil {
				return nil
			}
			dlqTopic := topicForEventType(envelope.EventType) + cfg.Kafka.DLQSuffix
			copyEnv := *envelope
			copyEnv.Metadata = make(map[string]string, len(envelope.Metadata)+1)
			for k, v := range envelope.Metadata {
				copyEnv.Metadata[k] = v
			}
			if reason != nil {
				copyEnv.Metadata["dlq_error"] = reason.Error()
			}
			return producer.Publish(pCtx, dlqTopic, envelope.EventID, &copyEnv)
		})

		topics := []string{
			events.TopicUserRegistered,
			events.TopicStoreApproved,
			events.TopicStoreRejected,
			events.TopicMerchantApproved,
			events.TopicMerchantActivated,
			events.TopicMerchantRejected,
			events.TopicMerchantSuspended,
			events.TopicOrderCreated,
			events.TopicOrderConfirmed,
			events.TopicOrderPaymentConfirmed,
			events.TopicOrderShipped,
			events.TopicOrderDelivered,
			events.TopicOrderCancelled,
			events.TopicOrderPaymentFailed,
			events.TopicPaymentSuccessful,
			events.TopicPaymentFailed,
			events.TopicDeliveryCreated,
			events.TopicDeliveryStatusUpdated,
			events.TopicReturnApproved,
			events.TopicReturnRejected,
			events.TopicRefundCompleted,
		}

		kafkaConsumer := kafka.NewConsumer(
			kafka.Config{
				Brokers:        cfg.Kafka.Brokers,
				ClientID:       cfg.Kafka.ClientID,
				ConnectTimeout: 10 * time.Second,
				MaxRetries:     cfg.Kafka.MaxRetries,
				RetryInterval:  cfg.Kafka.RetryInterval,
				DLQSuffix:      cfg.Kafka.DLQSuffix,
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

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcclient.UnaryServerInterceptor(),
		),
	)
	notificationpb.RegisterNotificationServiceServer(grpcServer, grpcHandler)
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
		log.Info("HTTP observability server listening", "service", cfg.App.Name, "port", cfg.HTTP.Port)
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

func topicForEventType(eventType string) string {
	switch eventType {
	case events.EventTypeUserRegistered:
		return events.TopicUserRegistered
	case events.EventTypeStoreApproved:
		return events.TopicStoreApproved
	case events.EventTypeStoreRejected:
		return events.TopicStoreRejected
	case events.EventTypeMerchantApproved:
		return events.TopicMerchantApproved
	case events.EventTypeMerchantActivated:
		return events.TopicMerchantActivated
	case events.EventTypeMerchantRejected:
		return events.TopicMerchantRejected
	case events.EventTypeMerchantSuspended:
		return events.TopicMerchantSuspended
	case events.EventTypeOrderCreated:
		return events.TopicOrderCreated
	case events.EventTypeOrderConfirmed:
		return events.TopicOrderConfirmed
	case events.EventTypeOrderPaymentConfirmed:
		return events.TopicOrderPaymentConfirmed
	case events.EventTypeOrderShipped:
		return events.TopicOrderShipped
	case events.EventTypeOrderDelivered:
		return events.TopicOrderDelivered
	case events.EventTypeOrderCancelled:
		return events.TopicOrderCancelled
	case events.EventTypeOrderPaymentFailed:
		return events.TopicOrderPaymentFailed
	case events.EventTypePaymentSuccessful:
		return events.TopicPaymentSuccessful
	case events.EventTypePaymentFailed:
		return events.TopicPaymentFailed
	case events.EventTypeDeliveryCreated:
		return events.TopicDeliveryCreated
	case events.EventTypeDeliveryStatusUpdated:
		return events.TopicDeliveryStatusUpdated
	case events.EventTypeDeliveryStatusChanged:
		return events.TopicDeliveryStatusChanged
	case events.EventTypeReturnApproved:
		return events.TopicReturnApproved
	case events.EventTypeReturnRejected:
		return events.TopicReturnRejected
	case events.EventTypeRefundCompleted:
		return events.TopicRefundCompleted
	default:
		return "gocart.notification.events"
	}
}
