package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/google/uuid"
	paymentpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/payment"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/pkg/health"
	"github.com/marees-godev/GoCart-Server/pkg/kafka"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/metrics"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/gateway"
	paymentGRPC "github.com/marees-godev/GoCart-Server/services/payment-service/internal/grpc"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/service"
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

	// 5. Initialize Outbox Publisher background worker for Kafka event streaming
	kafkaProducer := kafka.NewProducer(kafka.Config{
		Brokers: cfg.Kafka.Brokers,
	})
	defer kafkaProducer.Close()

	outboxPublisher := outbox.NewPublisher(db.Pool, kafkaProducer, outbox.NewStore(), outbox.DefaultConfig())
	go outboxPublisher.Start(ctx)

	// 6. Initialize domain dependencies & Payment Service
	paymentRepo := repository.NewPaymentRepository(db.Pool)

	gwConfig := gateway.GatewayConfig{
		Provider:          cfg.Gateway.Provider,
		APIKey:            cfg.Gateway.APIKey,
		RazorpayKeyID:     cfg.Gateway.RazorpayKeyID,
		RazorpayKeySecret: cfg.Gateway.RazorpayKeySecret,
		Timeout:           time.Duration(cfg.Gateway.Timeout) * time.Second,
	}
	paymentGW := gateway.NewPaymentGateway(gwConfig)

	orderClient := client.NewOrderClient(cfg.Services.OrderServiceURL)
	paymentService := service.NewPaymentService(paymentRepo, paymentGW, orderClient, gwConfig.Timeout)

	paymentGRPCServer := paymentGRPC.NewPaymentGRPCServer(paymentService)

	// 7. Setup gRPC server
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpcclient.UnaryServerInterceptor()),
		grpc.MaxRecvMsgSize(10*1024*1024),
		grpc.MaxSendMsgSize(10*1024*1024),
	)
	paymentpb.RegisterPaymentServiceServer(grpcServer, paymentGRPCServer)
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

	// 8. Setup Fiber HTTP server for observability (/health, /ready, /metrics)
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

	app.Get("/get-key", func(c *fiber.Ctx) error {
		key := gwConfig.RazorpayKeyID
		if key == "" {
			key = "rzp_test_xxxx"
		}
		return c.JSON(fiber.Map{"key": key})
	})

	app.Post("/create-order", func(c *fiber.Ctx) error {
		type orderReq struct {
			Amount   float64 `json:"amount"`
			Currency string  `json:"currency"`
		}
		var req orderReq
		_ = c.BodyParser(&req)
		if req.Amount <= 0 {
			req.Amount = 1499
		}
		if req.Currency == "" {
			req.Currency = "INR"
		}

		// If real Razorpay key & secret are provided, create genuine Razorpay Order ID via API
		if gwConfig.RazorpayKeyID != "" && gwConfig.RazorpayKeySecret != "" && !strings.Contains(gwConfig.RazorpayKeyID, "xxxx") {
			paymentID := uuid.New().String()
			orderID := uuid.New().String()
			userID := uuid.New().String()
			idempKey := fmt.Sprintf("idemp_%d", time.Now().UnixNano())

			rzpResp, err := paymentGW.ProcessPayment(c.UserContext(), &gateway.ProcessGatewayRequest{
				PaymentID:      paymentID,
				OrderID:        orderID,
				Amount:         req.Amount,
				Currency:       req.Currency,
				IdempotencyKey: idempKey,
			})

			if err == nil && rzpResp != nil && rzpResp.GatewayTransactionID != "" {
				pmt := &model.Payment{
					ID:                   paymentID,
					OrderID:              orderID,
					UserID:               userID,
					PaymentMethod:        "CREDIT_CARD",
					Amount:               req.Amount,
					Currency:             req.Currency,
					Status:               model.PaymentStatusPending,
					TransactionID:        fmt.Sprintf("tx_%s", uuid.New().String()),
					GatewayTransactionID: rzpResp.GatewayTransactionID,
					IdempotencyKey:       idempKey,
				}
				if createErr := paymentRepo.CreatePayment(c.UserContext(), pmt); createErr != nil {
					log.Error("failed to persist payment record for razorpay order", "error", createErr)
				} else {
					log.Info("persisted pending razorpay payment in database", "payment_id", pmt.ID, "razorpay_order_id", rzpResp.GatewayTransactionID)
				}

				return c.JSON(fiber.Map{
					"id":         rzpResp.GatewayTransactionID,
					"payment_id": paymentID,
					"amount":     int64(req.Amount * 100),
					"currency":   req.Currency,
					"status":     "created",
					"is_real":    true,
				})
			}
		}

		// If test / placeholder mode, leave order ID empty so Razorpay SDK allows standard client-side checkout
		return c.JSON(fiber.Map{
			"id":       "",
			"amount":   int64(req.Amount * 100),
			"currency": req.Currency,
			"status":   "created",
			"is_real":  false,
		})
	})

	app.All("/payment-callback", func(c *fiber.Ctx) error {
		orderID := c.FormValue("razorpay_order_id", c.Query("razorpay_order_id", c.Query("orderId", "")))
		paymentID := c.FormValue("razorpay_payment_id", c.Query("razorpay_payment_id", c.Query("paymentId", "")))
		signature := c.FormValue("razorpay_signature", c.Query("razorpay_signature", c.Query("signature", "")))

		return c.JSON(fiber.Map{
			"status":     "success",
			"order_id":   orderID,
			"payment_id": paymentID,
			"signature":  signature,
		})
	})

	// Webhook endpoints
	webhookHandler := handler.NewWebhookHandler(paymentService, gwConfig.RazorpayWebhookSecret)
	webhookHandler.RegisterRoutes(app)

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
