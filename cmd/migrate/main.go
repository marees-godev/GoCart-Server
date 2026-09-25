package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/marees-godev/GoCart-Server/pkg/database"
)

type serviceDef struct {
	Name           string
	Aliases        []string
	ServiceDir     string
	EnvKey         string
	MigrationsPath string
}

var services = []serviceDef{
	{Name: "auth", Aliases: []string{"auth", "auth-service"}, ServiceDir: "auth-service", EnvKey: "AUTH_SERVICE_DATABASE_URL", MigrationsPath: "services/auth-service/migrations"},
	{Name: "user", Aliases: []string{"user", "users", "user-service"}, ServiceDir: "user-service", EnvKey: "USER_SERVICE_DATABASE_URL", MigrationsPath: "services/user-service/migrations"},
	{Name: "merchant", Aliases: []string{"merchant", "merchants", "merchant-service"}, ServiceDir: "merchant-service", EnvKey: "MERCHANT_SERVICE_DATABASE_URL", MigrationsPath: "services/merchant-service/migrations"},
	{Name: "store", Aliases: []string{"store", "stores", "store-service"}, ServiceDir: "store-service", EnvKey: "STORE_SERVICE_DATABASE_URL", MigrationsPath: "services/store-service/migrations"},
	{Name: "category", Aliases: []string{"category", "categories", "category-service"}, ServiceDir: "category-service", EnvKey: "CATEGORY_SERVICE_DATABASE_URL", MigrationsPath: "services/category-service/migrations"},
	{Name: "product", Aliases: []string{"product", "products", "product-service"}, ServiceDir: "product-service", EnvKey: "PRODUCT_SERVICE_DATABASE_URL", MigrationsPath: "services/product-service/migrations"},
	{Name: "inventory", Aliases: []string{"inventory", "inventories", "inventory-service"}, ServiceDir: "inventory-service", EnvKey: "INVENTORY_SERVICE_DATABASE_URL", MigrationsPath: "services/inventory-service/migrations"},
	{Name: "cart", Aliases: []string{"cart", "carts", "cart-service"}, ServiceDir: "cart-service", EnvKey: "CART_SERVICE_DATABASE_URL", MigrationsPath: "services/cart-service/migrations"},
	{Name: "order", Aliases: []string{"order", "orders", "order-service"}, ServiceDir: "order-service", EnvKey: "ORDER_SERVICE_DATABASE_URL", MigrationsPath: "services/order-service/migrations"},
	{Name: "payment", Aliases: []string{"payment", "payments", "payment-service"}, ServiceDir: "payment-service", EnvKey: "PAYMENT_SERVICE_DATABASE_URL", MigrationsPath: "services/payment-service/migrations"},
	{Name: "delivery", Aliases: []string{"delivery", "deliveries", "delivery-service"}, ServiceDir: "delivery-service", EnvKey: "DELIVERY_SERVICE_DATABASE_URL", MigrationsPath: "services/delivery-service/migrations"},
	{Name: "return", Aliases: []string{"return", "returns", "return-service"}, ServiceDir: "return-service", EnvKey: "RETURN_SERVICE_DATABASE_URL", MigrationsPath: "services/return-service/migrations"},
	{Name: "rating", Aliases: []string{"rating", "ratings", "rating-service"}, ServiceDir: "rating-service", EnvKey: "RATINGS_SERVICE_DATABASE_URL", MigrationsPath: "services/rating-service/migrations"},
	{Name: "notification", Aliases: []string{"notification", "notifications", "notification-service"}, ServiceDir: "notification-service", EnvKey: "NOTIFICATIONS_SERVICE_DATABASE_URL", MigrationsPath: "services/notification-service/migrations"},
}

func main() {
	serviceFlag := flag.String("service", "", "Target service name (e.g. auth, user, orders) or 'all'")
	actionFlag := flag.String("action", "up", "Migration action: up, drop, reset")
	flag.Parse()

	_ = godotenv.Load(".env")

	action := strings.ToLower(strings.TrimSpace(*actionFlag))
	if action != "up" && action != "drop" && action != "reset" {
		fmt.Printf("Invalid action: %s. Valid actions: up, drop, reset\n", *actionFlag)
		os.Exit(1)
	}

	if *serviceFlag == "" {
		fmt.Println("Usage: go run cmd/migrate/main.go -service=<name|all> [-action=up|drop|reset]")
		fmt.Println("\nAvailable actions:")
		fmt.Println("  - up    : Apply pending migrations (default)")
		fmt.Println("  - drop  : Drop all tables and reset public schema")
		fmt.Println("  - reset : Drop all tables and apply migrations from scratch")
		fmt.Println("\nAvailable services:")
		for _, s := range services {
			fmt.Printf("  - %-14s (aliases: %s)\n", s.Name, strings.Join(s.Aliases, ", "))
		}
		os.Exit(1)
	}

	var targets []serviceDef
	if strings.EqualFold(*serviceFlag, "all") {
		targets = services
	} else {
		for _, s := range services {
			if matchesService(s, *serviceFlag) {
				targets = append(targets, s)
				break
			}
		}
		if len(targets) == 0 {
			slog.Error("Unknown service", "service", *serviceFlag)
			os.Exit(1)
		}
	}

	ctx := context.Background()
	hasErrors := false

	for _, target := range targets {
		fmt.Printf("\n=== %s Service: %s ===\n", strings.ToUpper(action), strings.ToUpper(target.Name))

		dbURL := getServiceDatabaseURL(target)

		if dbURL == "" {
			slog.Error("Missing database URL", "service", target.Name)
			hasErrors = true
			continue
		}

		cfg := database.DefaultConfig()
		cfg.URL = dbURL
		cfg.ConnectTimeout = 25 * time.Second
		cfg.MaxRetries = 4
		cfg.RetryInterval = 2 * time.Second

		db, err := database.New(ctx, cfg)
		if err != nil {
			slog.Error("Database connection failed", "service", target.Name, "error", err)
			hasErrors = true
			continue
		}

		migrator, err := database.NewMigrator(db.Pool, target.MigrationsPath)
		if err != nil {
			slog.Error("Failed to initialize migrator", "service", target.Name, "error", err)
			db.Close()
			hasErrors = true
			continue
		}

		if action == "drop" || action == "reset" {
			if err := migrator.DropAll(ctx); err != nil {
				slog.Error("Failed to drop database tables", "service", target.Name, "error", err)
				db.Close()
				hasErrors = true
				continue
			}
			slog.Info("Dropped all database tables successfully", "service", target.Name)
		}

		if action == "up" || action == "reset" {
			if err := migrator.Run(ctx); err != nil {
				slog.Error("Migration failed", "service", target.Name, "error", err)
				hasErrors = true
			} else {
				slog.Info("Migration finished successfully", "service", target.Name)
			}
		}

		db.Close()
	}

	if hasErrors {
		os.Exit(1)
	}
}

func matchesService(s serviceDef, input string) bool {
	if strings.EqualFold(s.Name, input) {
		return true
	}
	for _, alias := range s.Aliases {
		if strings.EqualFold(alias, input) {
			return true
		}
	}
	return false
}

func getServiceDatabaseURL(target serviceDef) string {
	keysToTry := []string{target.EnvKey}
	legacyKey := strings.Replace(target.EnvKey, "_SERVICE_DATABASE_URL", "_DATABASE_URL", 1)
	if legacyKey != target.EnvKey {
		keysToTry = append(keysToTry, legacyKey)
	}

	for _, k := range keysToTry {
		if val := os.Getenv(k); val != "" {
			return val
		}
	}

	envPath := filepath.Join("services", target.ServiceDir, ".env")
	if envMap, err := godotenv.Read(envPath); err == nil {
		for _, k := range keysToTry {
			if val := envMap[k]; val != "" {
				return val
			}
		}
		if val := envMap["DATABASE_URL"]; val != "" {
			return val
		}
	}

	return ""
}
