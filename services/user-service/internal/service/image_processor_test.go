package service_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/service"
)

type mockStorageUploader struct {
	uploadBytesFunc func(ctx context.Context, key string, data []byte, contentType string) (string, error)
	generateKeyFunc func(prefix, filename string) string
	lastUploadedKey string
	lastData        []byte
	lastContentType string
}

func (m *mockStorageUploader) Upload(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	data, _ := io.ReadAll(body)
	return m.UploadBytes(ctx, key, data, contentType)
}

func (m *mockStorageUploader) UploadBytes(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	m.lastUploadedKey = key
	m.lastData = data
	m.lastContentType = contentType
	if m.uploadBytesFunc != nil {
		return m.uploadBytesFunc(ctx, key, data, contentType)
	}
	return "https://storage.example.com/" + key, nil
}

func (m *mockStorageUploader) Delete(ctx context.Context, key string) error {
	return nil
}

func (m *mockStorageUploader) GetPublicURL(key string) string {
	return "https://storage.example.com/" + key
}

func (m *mockStorageUploader) GenerateKey(prefix, filename string) string {
	if m.generateKeyFunc != nil {
		return m.generateKeyFunc(prefix, filename)
	}
	return prefix + "/" + filename
}

func (m *mockStorageUploader) GetPresignedPutURL(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error) {
	return "", nil
}

func createTestPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func createTestJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 50, G: 150, B: 250, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	return buf.Bytes()
}

func TestImageProcessor_ValidPNG(t *testing.T) {
	uploader := &mockStorageUploader{}
	proc := service.NewImageProcessor(uploader)

	pngBytes := createTestPNG(120, 120)
	input := service.AvatarInput{
		Data:        pngBytes,
		ContentType: "image/png",
		Filename:    "my-avatar.png",
	}

	url, err := proc.ProcessAndUploadAvatar(context.Background(), "user-123", input)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	expectedPrefix := "https://storage.example.com/avatars/user-123/"
	if !strings.HasPrefix(url, expectedPrefix) {
		t.Errorf("expected url starting with %s, got %s", expectedPrefix, url)
	}

	if uploader.lastContentType != "image/png" {
		t.Errorf("expected content type image/png, got %s", uploader.lastContentType)
	}
}

func TestImageProcessor_ValidJPEG(t *testing.T) {
	uploader := &mockStorageUploader{}
	proc := service.NewImageProcessor(uploader)

	jpgBytes := createTestJPEG(200, 200)
	input := service.AvatarInput{
		Data:        jpgBytes,
		ContentType: "image/jpeg",
		Filename:    "photo.jpeg",
	}

	url, err := proc.ProcessAndUploadAvatar(context.Background(), "user-456", input)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !strings.Contains(url, "user-456") {
		t.Errorf("expected url containing user id, got %s", url)
	}

	if uploader.lastContentType != "image/jpeg" {
		t.Errorf("expected content type image/jpeg, got %s", uploader.lastContentType)
	}
}

func TestImageProcessor_LargeImageDownscaled(t *testing.T) {
	uploader := &mockStorageUploader{}
	proc := service.NewImageProcessor(uploader)

	largePNG := createTestPNG(1200, 800)
	input := service.AvatarInput{
		Data:        largePNG,
		ContentType: "image/png",
		Filename:    "large.png",
	}

	_, err := proc.ProcessAndUploadAvatar(context.Background(), "user-789", input)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// Verify decoded image dimensions
	decoded, _, err := image.Decode(bytes.NewReader(uploader.lastData))
	if err != nil {
		t.Fatalf("failed to decode uploaded image: %v", err)
	}

	bounds := decoded.Bounds()
	if bounds.Dx() > service.MaxAvatarDimension || bounds.Dy() > service.MaxAvatarDimension {
		t.Errorf("expected bounds <= %d, got %dx%d", service.MaxAvatarDimension, bounds.Dx(), bounds.Dy())
	}

	// Aspect ratio 1200:800 = 3:2, so max 512 width -> height 341
	if bounds.Dx() != 512 {
		t.Errorf("expected width 512, got %d", bounds.Dx())
	}
	if bounds.Dy() != 341 {
		t.Errorf("expected height 341, got %d", bounds.Dy())
	}
}

func TestImageProcessor_SmallImageNotUpscaled(t *testing.T) {
	uploader := &mockStorageUploader{}
	proc := service.NewImageProcessor(uploader)

	smallPNG := createTestPNG(150, 100)
	input := service.AvatarInput{
		Data:        smallPNG,
		ContentType: "image/png",
	}

	_, err := proc.ProcessAndUploadAvatar(context.Background(), "user-100", input)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	decoded, _, err := image.Decode(bytes.NewReader(uploader.lastData))
	if err != nil {
		t.Fatalf("failed to decode uploaded image: %v", err)
	}

	bounds := decoded.Bounds()
	if bounds.Dx() != 150 || bounds.Dy() != 100 {
		t.Errorf("expected original dimensions 150x100, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestImageProcessor_EmptyData(t *testing.T) {
	uploader := &mockStorageUploader{}
	proc := service.NewImageProcessor(uploader)

	_, err := proc.ProcessAndUploadAvatar(context.Background(), "user-1", service.AvatarInput{
		Data: []byte{},
	})
	if err == nil {
		t.Fatal("expected error for empty data, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST code, got %s", appErr.Code)
	}
}

func TestImageProcessor_FileTooLarge(t *testing.T) {
	uploader := &mockStorageUploader{}
	proc := service.NewImageProcessor(uploader)

	hugeData := make([]byte, service.MaxAvatarSizeBytes+1024)
	_, err := proc.ProcessAndUploadAvatar(context.Background(), "user-1", service.AvatarInput{
		Data: hugeData,
	})
	if err == nil {
		t.Fatal("expected error for file too large, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST code, got %s", appErr.Code)
	}
	if !strings.Contains(appErr.Message, "file too large") {
		t.Errorf("expected 'file too large' message, got %s", appErr.Message)
	}
}

func TestImageProcessor_InvalidFileType(t *testing.T) {
	uploader := &mockStorageUploader{}
	proc := service.NewImageProcessor(uploader)

	tests := []struct {
		name string
		data []byte
	}{
		{"plain text", []byte("this is plain text not an image")},
		{"pdf header", []byte("%PDF-1.4 header contents")},
		{"random bytes", []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := proc.ProcessAndUploadAvatar(context.Background(), "user-1", service.AvatarInput{
				Data: tt.data,
			})
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}

			appErr := appErrors.AsAppError(err)
			if appErr.Code != appErrors.CodeBadRequest {
				t.Errorf("expected BAD_REQUEST code, got %s", appErr.Code)
			}
			if !strings.Contains(appErr.Message, "invalid file type") {
				t.Errorf("expected 'invalid file type' message, got %s", appErr.Message)
			}
		})
	}
}

func TestImageProcessor_CorruptedImage(t *testing.T) {
	uploader := &mockStorageUploader{}
	proc := service.NewImageProcessor(uploader)

	// Valid PNG signature (8 bytes) followed by corrupt garbage
	corruptPNG := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0xFF, 0xFE}

	_, err := proc.ProcessAndUploadAvatar(context.Background(), "user-1", service.AvatarInput{
		Data: corruptPNG,
	})
	if err == nil {
		t.Fatal("expected error for corrupted image, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST code, got %s", appErr.Code)
	}
	if !strings.Contains(appErr.Message, "corrupted") && !strings.Contains(appErr.Message, "invalid") {
		t.Errorf("expected invalid/corrupted message, got %s", appErr.Message)
	}
}

func TestImageProcessor_NilUploader(t *testing.T) {
	proc := service.NewImageProcessor(nil)
	pngBytes := createTestPNG(50, 50)

	_, err := proc.ProcessAndUploadAvatar(context.Background(), "user-1", service.AvatarInput{
		Data: pngBytes,
	})
	if err == nil {
		t.Fatal("expected error for nil uploader, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeInternalError {
		t.Errorf("expected INTERNAL code, got %s", appErr.Code)
	}
}

func TestImageProcessor_StorageUploadFailure(t *testing.T) {
	uploader := &mockStorageUploader{
		uploadBytesFunc: func(ctx context.Context, key string, data []byte, contentType string) (string, error) {
			return "", errors.New("s3 connection timeout")
		},
	}
	proc := service.NewImageProcessor(uploader)
	pngBytes := createTestPNG(50, 50)

	_, err := proc.ProcessAndUploadAvatar(context.Background(), "user-1", service.AvatarInput{
		Data: pngBytes,
	})
	if err == nil {
		t.Fatal("expected error for storage upload failure, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeInternalError {
		t.Errorf("expected INTERNAL code, got %s", appErr.Code)
	}
	if !strings.Contains(appErr.Message, "failed to store avatar image") {
		t.Errorf("expected storage error message, got %s", appErr.Message)
	}
}

func TestParseDataURI(t *testing.T) {
	rawPNG := createTestPNG(10, 10)
	b64PNG := base64.StdEncoding.EncodeToString(rawPNG)
	validURI := "data:image/png;base64," + b64PNG

	decoded, ct, err := service.ParseDataURI(validURI)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ct != "image/png" {
		t.Errorf("expected content type image/png, got %s", ct)
	}
	if len(decoded) != len(rawPNG) {
		t.Errorf("expected length %d, got %d", len(rawPNG), len(decoded))
	}

	// Invalid prefix
	_, _, err = service.ParseDataURI("https://example.com/image.png")
	if err == nil {
		t.Fatal("expected error for non-data URI")
	}

	// Malformed data URI
	_, _, err = service.ParseDataURI("data:image/png;base64")
	if err == nil {
		t.Fatal("expected error for missing comma")
	}

	// Invalid base64
	_, _, err = service.ParseDataURI("data:image/png;base64,invalid!base64!data")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}
