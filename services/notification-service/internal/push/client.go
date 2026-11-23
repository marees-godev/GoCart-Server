package push

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
)

var (
	ErrVAPIDNotConfigured = errors.New("VAPID keys are not configured")
	ErrSubscriptionGone   = errors.New("push subscription is expired or gone")
)

type SendResult struct {
	StatusCode       int
	ResponseBody     string
	Attempts         int
	SubscriptionGone bool
	Retryable        bool
}

type Sender interface {
	Send(ctx context.Context, sub *model.PushSubscription, payload []byte) (*SendResult, error)
	PublicKey() string
	Subject() string
	IsConfigured() bool
}

type WebPushClient struct {
	vapid         config.VAPIDConfig
	httpClient    *http.Client
	maxRetries    int
	retryInterval time.Duration
	ttlSeconds    int
	logger        *slog.Logger
}

func NewWebPushClient(
	vapid config.VAPIDConfig,
	pushCfg config.PushConfig,
	httpClient *http.Client,
	log *slog.Logger,
) *WebPushClient {
	if httpClient == nil {
		timeout := pushCfg.RequestTimeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	maxRetries := pushCfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}
	retryInterval := pushCfg.RetryInterval
	if retryInterval <= 0 {
		retryInterval = 200 * time.Millisecond
	}
	ttlSeconds := pushCfg.TTLSeconds
	if ttlSeconds <= 0 {
		ttlSeconds = 86400
	}
	if log == nil {
		log = slog.Default()
	}

	return &WebPushClient{
		vapid:         vapid,
		httpClient:    httpClient,
		maxRetries:    maxRetries,
		retryInterval: retryInterval,
		ttlSeconds:    ttlSeconds,
		logger:        log,
	}
}

func (c *WebPushClient) PublicKey() string {
	return strings.TrimSpace(c.vapid.PublicKey)
}

func (c *WebPushClient) Subject() string {
	sub := strings.TrimSpace(c.vapid.Subject)
	if sub == "" {
		return "mailto:notifications@gocart.local"
	}
	return sub
}

func (c *WebPushClient) IsConfigured() bool {
	return c.vapid.IsConfigured()
}

func (c *WebPushClient) Send(ctx context.Context, sub *model.PushSubscription, payload []byte) (*SendResult, error) {
	if sub == nil {
		return nil, errors.New("push subscription is nil")
	}
	if !c.IsConfigured() {
		return nil, ErrVAPIDNotConfigured
	}

	authHeader, err := BuildVAPIDAuthHeader(
		sub.Endpoint,
		c.vapid.Subject,
		c.vapid.PublicKey,
		c.vapid.PrivateKey,
		defaultVAPIDTTL,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed building VAPID auth header: %w", err)
	}

	encryptedBody, err := EncryptPayload(payload, sub.P256DH, sub.Auth)
	if err != nil {
		return nil, fmt.Errorf("failed encrypting push payload: %w", err)
	}

	var lastResult *SendResult
	var lastErr error
	backoff := c.retryInterval

	for attempt := 1; attempt <= c.maxRetries; attempt++ {
		if ctx.Err() != nil {
			return lastResult, ctx.Err()
		}

		res, reqErr := c.doPost(ctx, sub.Endpoint, authHeader, encryptedBody)
		if res != nil {
			res.Attempts = attempt
		}
		lastResult = res

		if reqErr == nil {
			return res, nil
		}
		lastErr = reqErr

		if res != nil && res.SubscriptionGone {
			return res, fmt.Errorf("%w (status=%d)", ErrSubscriptionGone, res.StatusCode)
		}

		if res == nil || !res.Retryable || attempt == c.maxRetries {
			return res, reqErr
		}

		c.logger.WarnContext(ctx, "Transient Web Push delivery failure, scheduling retry",
			"subscription_id", sub.ID,
			"attempt", attempt,
			"max_retries", c.maxRetries,
			"status_code", res.StatusCode,
			"error", reqErr,
		)

		select {
		case <-ctx.Done():
			return lastResult, ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
		}
	}

	return lastResult, lastErr
}

func (c *WebPushClient) doPost(ctx context.Context, endpoint, authHeader string, body []byte) (*SendResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return &SendResult{Retryable: false}, fmt.Errorf("invalid push request: %w", err)
	}

	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", strconv.Itoa(c.ttlSeconds))
	req.Header.Set("Urgency", "normal")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &SendResult{Retryable: true}, fmt.Errorf("push HTTP transport error: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	respBody := strings.TrimSpace(string(respBytes))

	result := &SendResult{
		StatusCode:   resp.StatusCode,
		ResponseBody: respBody,
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return result, nil
	}

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		result.SubscriptionGone = true
		result.Retryable = false
		return result, fmt.Errorf("push endpoint returned %d", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		result.Retryable = true
		return result, fmt.Errorf("transient push endpoint error %d: %s", resp.StatusCode, respBody)
	}

	result.Retryable = false
	return result, fmt.Errorf("push endpoint rejected notification (%d): %s", resp.StatusCode, respBody)
}

