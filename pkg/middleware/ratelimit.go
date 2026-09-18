package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/response"
)

type RateLimiterConfig struct {
	MaxRequests int
	Window      time.Duration
	KeyFunc     func(r *http.Request) string
	Message     string
}

type clientRecord struct {
	count     int
	resetTime time.Time
}

type RateLimiter struct {
	mu          sync.Mutex
	records     map[string]*clientRecord
	maxRequests int
	window      time.Duration
	keyFunc     func(r *http.Request) string
	message     string
}

func DefaultKeyFunc(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return xrip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func NewRateLimiter(cfg RateLimiterConfig) *RateLimiter {
	if cfg.MaxRequests <= 0 {
		cfg.MaxRequests = 100
	}
	if cfg.Window <= 0 {
		cfg.Window = time.Minute
	}
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = DefaultKeyFunc
	}
	if cfg.Message == "" {
		cfg.Message = "Too many requests, please try again later."
	}

	return &RateLimiter{
		records:     make(map[string]*clientRecord),
		maxRequests: cfg.MaxRequests,
		window:      cfg.Window,
		keyFunc:     cfg.KeyFunc,
		message:     cfg.Message,
	}
}

func RateLimit(cfg RateLimiterConfig) func(http.Handler) http.Handler {
	rl := NewRateLimiter(cfg)
	return rl.Middleware
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		key := rl.keyFunc(r)
		now := time.Now()

		rl.mu.Lock()
		rec, exists := rl.records[key]
		if !exists || now.After(rec.resetTime) {
			rec = &clientRecord{
				count:     1,
				resetTime: now.Add(rl.window),
			}
			rl.records[key] = rec
			rl.mu.Unlock()
			next.ServeHTTP(w, r)
			return
		}

		if rec.count >= rl.maxRequests {
			retryAfter := int(time.Until(rec.resetTime).Seconds()) + 1
			rl.mu.Unlock()
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			response.Error(w, http.StatusTooManyRequests, errors.CodeTooManyRequests, rl.message)
			return
		}

		rec.count++
		rl.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}
