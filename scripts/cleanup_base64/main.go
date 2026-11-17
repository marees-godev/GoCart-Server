package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := os.Getenv("PRODUCTS_DB_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		log.Fatal("PRODUCTS_DB_URL or DATABASE_URL environment variable is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping: %v", err)
	}
	fmt.Println("Connected to Products DB")

	// Audit: list Base64 rows
	rows, err := pool.Query(ctx, "SELECT id, LEFT(image_url, 60) FROM products WHERE image_url LIKE 'data:%'")
	if err != nil {
		log.Fatalf("query products: %v", err)
	}
	fmt.Println("\n=== products.image_url Base64 rows ===")
	count := 0
	for rows.Next() {
		var id, preview string
		rows.Scan(&id, &preview)
		fmt.Printf("  id=%s  url=%s...\n", id, preview)
		count++
	}
	rows.Close()
	fmt.Printf("Total: %d rows\n", count)

	rows2, err := pool.Query(ctx, "SELECT id, LEFT(url, 60) FROM product_images WHERE url LIKE 'data:%'")
	if err != nil {
		log.Fatalf("query product_images: %v", err)
	}
	fmt.Println("\n=== product_images.url Base64 rows ===")
	count2 := 0
	for rows2.Next() {
		var id, preview string
		rows2.Scan(&id, &preview)
		fmt.Printf("  id=%s  url=%s...\n", id, preview)
		count2++
	}
	rows2.Close()
	fmt.Printf("Total: %d rows\n", count2)

	if count == 0 && count2 == 0 {
		fmt.Println("\nNo Base64 rows found. Nothing to clean up.")
		return
	}

	// Nullify Base64 image_url in products
	tag, err := pool.Exec(ctx, "UPDATE products SET image_url = NULL WHERE image_url LIKE 'data:%'")
	if err != nil {
		log.Fatalf("update products: %v", err)
	}
	fmt.Printf("\nNullified %d rows in products.image_url\n", tag.RowsAffected())

	// Nullify Base64 url in product_images
	tag2, err := pool.Exec(ctx, "UPDATE product_images SET url = NULL WHERE url LIKE 'data:%'")
	if err != nil {
		log.Fatalf("update product_images: %v", err)
	}
	fmt.Printf("Nullified %d rows in product_images.url\n", tag2.RowsAffected())

	fmt.Println("\nCleanup complete.")
}
