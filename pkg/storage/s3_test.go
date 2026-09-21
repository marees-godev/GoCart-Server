package storage_test

import (
	"context"
	"strings"
	"testing"

	"github.com/marees-godev/GoCart-Server/pkg/storage"
)

func TestNewS3Client_Validation(t *testing.T) {
	_, err := storage.NewS3Client(context.Background(), storage.Config{})
	if err == nil {
		t.Fatal("expected error when endpoint is empty, got nil")
	}

	_, err = storage.NewS3Client(context.Background(), storage.Config{
		Endpoint: "https://test.supabase.co",
	})
	if err == nil {
		t.Fatal("expected error when credentials are empty, got nil")
	}
}

func TestS3Client_GenerateKeyAndPublicURL(t *testing.T) {
	client, err := storage.NewS3Client(context.Background(), storage.Config{
		Endpoint:        "https://cljkfzbiywvhzpmlbbuy.storage.supabase.co/storage/v1/s3",
		Region:          "ap-south-1",
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
		Bucket:          "stores",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	key := client.GenerateKey("logos", "my_store_logo.png")
	if !strings.HasPrefix(key, "logos/") || !strings.HasSuffix(key, ".png") {
		t.Errorf("unexpected key format: %s", key)
	}

	url := client.GetPublicURL("logos/logo-123.png")
	expectedPrefix := "https://cljkfzbiywvhzpmlbbuy.supabase.co/storage/v1/object/public/stores/logos/logo-123.png"
	if url != expectedPrefix {
		t.Errorf("expected %s, got %s", expectedPrefix, url)
	}

	presignedURL, err := client.GetPresignedPutURL(context.Background(), key, "image/png", 0)
	if err != nil {
		t.Fatalf("unexpected error generating presigned URL: %v", err)
	}
	if !strings.Contains(presignedURL, "cljkfzbiywvhzpmlbbuy") || !strings.Contains(presignedURL, "X-Amz-Signature") {
		t.Errorf("unexpected presigned URL format: %s", presignedURL)
	}
}
