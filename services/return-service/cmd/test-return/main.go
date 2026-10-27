package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/config"
	retEvents "github.com/marees-godev/GoCart-Server/services/return-service/internal/events"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/repository"
)

func main() {
	orderIDFlag := flag.String("order-id", "", "Order ID to return (required)")
	orderItemIDFlag := flag.String("item-id", "", "Order Item ID to return (required)")
	productIDFlag := flag.String("product-id", "", "Product ID of the item (optional)")
	userIDFlag := flag.String("user-id", "", "User ID of order owner (required)")
	refundFlag := flag.Float64("refund", 0.0, "Refund amount (required)")
	reasonFlag := flag.String("reason", "", "Reason for return (required)")
	flag.Parse()

	if *orderIDFlag == "" || *orderItemIDFlag == "" || *userIDFlag == "" || *reasonFlag == "" {
		fmt.Println("Usage of test-return:")
		fmt.Println("  go run ./cmd/test-return -order-id <uuid> -item-id <uuid> -user-id <uuid> -refund <amount> -reason <text>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	cfg := config.LoadEnv()
	if cfg.Database.URL == "" {
		fmt.Println("Error: RETURN_SERVICE_DATABASE_URL is empty")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	poolCfg, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		fmt.Printf("Failed to parse DB config: %v\n", err)
		os.Exit(1)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		fmt.Printf("Failed to create connection pool: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	db := &database.DB{Pool: pool}
	logger := slog.Default()
	outboxStore := outbox.NewStore()
	repo := repository.NewReturnRepository(db, outboxStore, logger)

	fmt.Printf("Checking if return already exists for Order Item: %s\n", *orderItemIDFlag)
	existing, err := repo.GetReturnByOrderItemID(ctx, *orderItemIDFlag)
	if err == nil && existing != nil {
		fmt.Println("\nExisting return found in database:")
		fmt.Printf("  Return ID:    %s\n", existing.ID)
		fmt.Printf("  Status:       %s\n", existing.Status)
		fmt.Printf("  Refund:       $%.2f\n", existing.RefundAmount)
		fmt.Printf("  Requested At: %s\n", existing.RequestedAt.Format(time.RFC3339))
		return
	}

	now := time.Now().UTC()
	returnID := uuid.New().String()
	ret := &model.Return{
		ID:           returnID,
		OrderID:      *orderIDFlag,
		OrderItemID:  *orderItemIDFlag,
		UserID:       *userIDFlag,
		Reason:       *reasonFlag,
		Status:       model.ReturnStatusRequested,
		RefundAmount: *refundFlag,
		RequestedAt:  now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	outboxEvt, err := retEvents.BuildReturnRequestedOutboxEvent(ret, *productIDFlag)
	if err != nil {
		fmt.Printf("Failed to build outbox event: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Inserting return into 'returns' table (ID: %s)...\n", ret.ID)
	err = repo.CreateReturnWithOutbox(ctx, ret, outboxEvt)
	if err != nil {
		fmt.Printf("Failed to insert return: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Return record committed to 'returns' table successfully!")

	fetched, err := repo.GetReturnByID(ctx, ret.ID)
	if err != nil {
		fmt.Printf("Failed to fetch back return: %v\n", err)
	} else {
		fmt.Println("\nVerified return record in 'returns' table:")
		fmt.Printf("  Return ID:    %s\n", fetched.ID)
		fmt.Printf("  Order ID:     %s\n", fetched.OrderID)
		fmt.Printf("  Item ID:      %s\n", fetched.OrderItemID)
		fmt.Printf("  User ID:      %s\n", fetched.UserID)
		fmt.Printf("  Status:       %s\n", fetched.Status)
		fmt.Printf("  Refund:       $%.2f\n", fetched.RefundAmount)
		fmt.Printf("  Reason:       %s\n", fetched.Reason)
		fmt.Printf("  Requested At: %s\n", fetched.RequestedAt.Format(time.RFC3339))
	}

	var count int
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM returns").Scan(&count)
	fmt.Printf("\nTotal return records currently in DB: %d\n", count)
}
