package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

type Config struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	PublicURLPrefix string
	UsePathStyle    bool
}

type Uploader interface {
	Upload(ctx context.Context, key string, body io.Reader, contentType string) (string, error)
	UploadBytes(ctx context.Context, key string, data []byte, contentType string) (string, error)
	Delete(ctx context.Context, key string) error
	GetPublicURL(key string) string
	GenerateKey(prefix, filename string) string
	GetPresignedPutURL(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error)
}

type S3Client struct {
	client          *s3.Client
	presignClient   *s3.PresignClient
	bucket          string
	publicURLPrefix string
}

func NewS3Client(ctx context.Context, cfg Config) (*S3Client, error) {
	if cfg.Endpoint == "" {
		return nil, appErrors.Internal(nil, "storage s3 endpoint is required")
	}
	if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, appErrors.Internal(nil, "storage s3 credentials are required")
	}
	if cfg.Region == "" {
		cfg.Region = "ap-south-1"
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "stores"
	}

	awsCfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(cfg.Region),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to configure S3 client")
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true
	})

	publicURLPrefix := strings.TrimRight(cfg.PublicURLPrefix, "/")
	if publicURLPrefix == "" {
		// Derive Supabase public storage url if supabase endpoint is used
		if strings.Contains(cfg.Endpoint, ".storage.supabase.co") {
			base := strings.Replace(cfg.Endpoint, ".storage.supabase.co/storage/v1/s3", ".supabase.co/storage/v1/object/public", 1)
			publicURLPrefix = fmt.Sprintf("%s/%s", strings.TrimRight(base, "/"), cfg.Bucket)
		} else {
			publicURLPrefix = fmt.Sprintf("%s/%s", strings.TrimRight(cfg.Endpoint, "/"), cfg.Bucket)
		}
	}

	presignClient := s3.NewPresignClient(client)

	return &S3Client{
		client:          client,
		presignClient:   presignClient,
		bucket:          cfg.Bucket,
		publicURLPrefix: publicURLPrefix,
	}, nil
}

func (s *S3Client) GetPresignedPutURL(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error) {
	if key == "" {
		return "", appErrors.BadRequest("storage key cannot be empty")
	}
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	req, err := s.presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", appErrors.Internal(err, "failed to generate presigned upload URL")
	}

	return req.URL, nil
}

func (s *S3Client) Upload(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	if key == "" {
		return "", appErrors.BadRequest("storage key cannot be empty")
	}

	var buf bytes.Buffer
	size, err := io.Copy(&buf, body)
	if err != nil {
		return "", appErrors.Internal(err, "failed to read upload payload")
	}

	if contentType == "" {
		contentType = http.DetectContentType(buf.Bytes())
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(buf.Bytes()),
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return "", appErrors.Internal(err, fmt.Sprintf("failed to upload object to S3: %s", key))
	}

	return s.GetPublicURL(key), nil
}

func (s *S3Client) UploadBytes(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	return s.Upload(ctx, key, bytes.NewReader(data), contentType)
}

func (s *S3Client) Delete(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return appErrors.Internal(err, fmt.Sprintf("failed to delete object from S3: %s", key))
	}
	return nil
}

func (s *S3Client) GetPublicURL(key string) string {
	cleanKey := strings.TrimLeft(key, "/")
	return fmt.Sprintf("%s/%s", s.publicURLPrefix, cleanKey)
}

func (s *S3Client) GenerateKey(prefix, filename string) string {
	ext := path.Ext(filename)
	cleanPrefix := strings.Trim(prefix, "/")
	timestamp := time.Now().Format("20060102")
	id := uuid.New().String()
	if cleanPrefix != "" {
		return fmt.Sprintf("%s/%s/%s%s", cleanPrefix, timestamp, id, ext)
	}
	return fmt.Sprintf("%s/%s%s", timestamp, id, ext)
}
