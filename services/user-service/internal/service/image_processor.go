package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/storage"
)

const (
	MaxAvatarSizeBytes = 5 * 1024 * 1024 // 5MB
	MaxAvatarDimension = 512             // 512px max dimension
	JPEGQuality        = 85
)

type AvatarInput struct {
	Data        []byte
	ContentType string
	Filename    string
}

type ImageProcessor interface {
	ProcessAndUploadAvatar(ctx context.Context, userID string, input AvatarInput) (string, error)
}

type defaultImageProcessor struct {
	uploader storage.Uploader
}

func NewImageProcessor(uploader storage.Uploader) ImageProcessor {
	return &defaultImageProcessor{uploader: uploader}
}

func (p *defaultImageProcessor) ProcessAndUploadAvatar(ctx context.Context, userID string, input AvatarInput) (string, error) {
	if p.uploader == nil {
		return "", errors.Internal(nil, "storage uploader is not configured")
	}

	data := input.Data
	if len(data) == 0 {
		return "", errors.BadRequest("avatar image cannot be empty")
	}

	if len(data) > MaxAvatarSizeBytes {
		return "", errors.BadRequest("file too large: avatar size must not exceed 5MB")
	}

	// 1. Detect and validate file type
	detectedType := http.DetectContentType(data)
	isJPEG := detectedType == "image/jpeg"
	isPNG := detectedType == "image/png"
	isWebP := strings.HasPrefix(detectedType, "image/webp") || (len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP")

	if !isJPEG && !isPNG && !isWebP {
		return "", errors.BadRequest("invalid file type: only JPEG, PNG, and WebP images are allowed")
	}

	// 2. Decode image and check for corruption
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", errors.BadRequest("invalid or corrupted image data")
	}

	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return "", errors.BadRequest("invalid or corrupted image data")
	}

	// 3. Resize image to avatar dimensions
	resized := resizeAvatar(img, MaxAvatarDimension)

	// 4. Encode image
	var buf bytes.Buffer
	var uploadContentType string
	var ext string

	switch strings.ToLower(format) {
	case "png":
		if err := png.Encode(&buf, resized); err != nil {
			return "", errors.Internal(err, "failed to process avatar image")
		}
		uploadContentType = "image/png"
		ext = ".png"
	case "jpeg", "jpg":
		if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: JPEGQuality}); err != nil {
			return "", errors.Internal(err, "failed to process avatar image")
		}
		uploadContentType = "image/jpeg"
		ext = ".jpg"
	case "webp":
		if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: JPEGQuality}); err != nil {
			return "", errors.Internal(err, "failed to process avatar image")
		}
		uploadContentType = "image/jpeg"
		ext = ".jpg"
	default:
		return "", errors.BadRequest("invalid file type: only JPEG, PNG, and WebP images are allowed")
	}

	// 5. Generate storage key
	filename := "avatar" + ext
	if input.Filename != "" {
		base := filepath.Base(input.Filename)
		cleanExt := filepath.Ext(base)
		if cleanExt != "" {
			filename = strings.TrimSuffix(base, cleanExt) + ext
		}
	}

	prefix := fmt.Sprintf("avatars/%s", userID)
	key := p.uploader.GenerateKey(prefix, filename)

	// 6. Upload to storage
	url, err := p.uploader.UploadBytes(ctx, key, buf.Bytes(), uploadContentType)
	if err != nil {
		return "", errors.Internal(err, "failed to store avatar image")
	}

	return url, nil
}

func resizeAvatar(src image.Image, maxDim int) image.Image {
	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w <= maxDim && h <= maxDim {
		return src
	}

	var newW, newH int
	if w > h {
		newW = maxDim
		newH = (h * maxDim) / w
	} else {
		newH = maxDim
		newW = (w * maxDim) / h
	}

	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}

func ParseDataURI(input string) ([]byte, string, error) {
	if !strings.HasPrefix(input, "data:image/") {
		return nil, "", errors.BadRequest("invalid image data URI format")
	}

	parts := strings.SplitN(input, ",", 2)
	if len(parts) != 2 {
		return nil, "", errors.BadRequest("invalid image data URI format")
	}

	header := parts[0]
	b64Data := parts[1]

	contentType := "image/png"
	if strings.Contains(header, "image/jpeg") || strings.Contains(header, "image/jpg") {
		contentType = "image/jpeg"
	} else if strings.Contains(header, "image/webp") {
		contentType = "image/webp"
	}

	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return nil, "", errors.BadRequest("invalid base64 image data")
	}

	return decoded, contentType, nil
}
