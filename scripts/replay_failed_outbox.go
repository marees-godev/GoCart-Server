package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
)

func main() {
	dbURL := flag.String("db", os.Getenv("MERCHANT_SERVICE_DATABASE_URL"), "PostgreSQL database connection URL")
	aggregateType := flag.String("aggregate", "", "Filter by aggregate type (optional)")
	eventType := flag.String("type", "", "Filter by event type (optional)")
	checkOnly := flag.Bool("check-only", false, "Inspect outbox counts without requeuing")
	flag.Parse()

	if *dbURL == "" {
		log.Fatal("Error: database URL must be provided via -db flag or MERCHANT_SERVICE_DATABASE_URL environment variable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, *dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	store := outbox.NewStore()

	// 1. Fetch current status summary
	counts, err := store.CountByStatus(ctx, pool)
	if err != nil {
		log.Fatalf("Failed to fetch outbox status counts: %v", err)
	}

	fmt.Println("=== Transactional Outbox Status ===")
	for status, count := range counts {
		fmt.Printf("  Status: %-10s Count: %d\n", status, count)
	}

	if *checkOnly {
		return
	}

	failedCount := counts[outbox.StatusFailed]
	if failedCount == 0 {
		fmt.Println("\nNo FAILED outbox events to replay.")
		return
	}

	// 2. Requeue failed events
	fmt.Printf("\nRequeuing %d FAILED events (aggregate: '%s', event_type: '%s')...\n", failedCount, *aggregateType, *eventType)
	requeued, err := store.RequeueFailed(ctx, pool, *aggregateType, *eventType)
	if err != nil {
		log.Fatalf("Failed to requeue failed events: %v", err)
	}

	fmt.Printf("Successfully requeued %d events back to PENDING.\n", requeued)
}
