package redis

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var (
	ErrNilClient     = errors.New("redis client is not initialized")
	ErrKeyNotFound   = errors.New("key not found in redis")
	ErrLockNotHeld   = errors.New("lock is not held or owned by caller")
)

const releaseLockLuaScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end
`

const incrWithExpiryLuaScript = `
local current = redis.call("incr", KEYS[1])
if current == 1 then
    redis.call("pexpire", KEYS[1], ARGV[1])
end
return current
`

type Client struct {
	rdb *goredis.Client
	cfg Config
}

func New(ctx context.Context, cfg Config) (*Client, error) {
	var opts *goredis.Options

	if cfg.URL != "" {
		parsedOpts, err := goredis.ParseURL(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("invalid redis url: %w", err)
		}
		opts = parsedOpts
	} else {
		opts = &goredis.Options{
			Addr:         cfg.Addr(),
			Password:     cfg.Password,
			DB:           cfg.DB,
			PoolSize:     cfg.PoolSize,
			MinIdleConns: cfg.MinIdleConns,
			DialTimeout:  cfg.DialTimeout,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		}
	}

	if cfg.EnableTLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	maxAttempts := cfg.MaxRetries
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	retryDelay := cfg.RetryInterval
	if retryDelay <= 0 {
		retryDelay = 1 * time.Second
	}

	sanitizedTarget := SanitizeAddr(opts.Addr, opts.Password)
	if cfg.URL != "" {
		sanitizedTarget = SanitizeURL(cfg.URL)
	}

	var rdb *goredis.Client
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		rdb = goredis.NewClient(opts)

		pingTimeout := 5 * time.Second
		if cfg.DialTimeout > 0 {
			pingTimeout = cfg.DialTimeout
		}
		pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
		lastErr = rdb.Ping(pingCtx).Err()
		cancel()

		if lastErr == nil {
			slog.Info("Connected to Redis",
				"target", sanitizedTarget,
				"db", opts.DB,
				"pool_size", opts.PoolSize,
			)
			return &Client{rdb: rdb, cfg: cfg}, nil
		}

		_ = rdb.Close()

		slog.Warn("Redis connection attempt failed",
			"attempt", attempt,
			"target", sanitizedTarget,
			"error", lastErr.Error(),
			"retry_in", retryDelay,
		)

		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during redis connection retry: %w", ctx.Err())
			case <-time.After(retryDelay):
				retryDelay *= 2
			}
		}
	}

	return nil, fmt.Errorf("failed to connect to redis after %d attempts: %w", maxAttempts, lastErr)
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.rdb == nil {
		return ErrNilClient
	}
	return c.rdb.Ping(ctx).Err()
}

func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

func (c *Client) RawClient() *goredis.Client {
	if c == nil {
		return nil
	}
	return c.rdb
}

func (c *Client) Config() Config {
	if c == nil {
		return Config{}
	}
	return c.cfg
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	if c == nil || c.rdb == nil {
		return "", ErrNilClient
	}
	val, err := c.rdb.Get(ctx, key).Result()
	if errors.Is(err, goredis.Nil) {
		return "", ErrKeyNotFound
	}
	return val, err
}

func (c *Client) GetBytes(ctx context.Context, key string) ([]byte, error) {
	if c == nil || c.rdb == nil {
		return nil, ErrNilClient
	}
	val, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, ErrKeyNotFound
	}
	return val, err
}

func (c *Client) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return ErrNilClient
	}
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *Client) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return ErrNilClient
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value to json: %w", err)
	}
	return c.Set(ctx, key, data, ttl)
}

func (c *Client) GetJSON(ctx context.Context, key string, dest any) error {
	data, err := c.GetBytes(ctx, key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("failed to unmarshal json from redis: %w", err)
	}
	return nil
}

func (c *Client) Delete(ctx context.Context, keys ...string) error {
	if c == nil || c.rdb == nil {
		return ErrNilClient
	}
	if len(keys) == 0 {
		return nil
	}
	return c.rdb.Del(ctx, keys...).Err()
}

func (c *Client) Exists(ctx context.Context, keys ...string) (bool, error) {
	if c == nil || c.rdb == nil {
		return false, ErrNilClient
	}
	if len(keys) == 0 {
		return false, nil
	}
	count, err := c.rdb.Exists(ctx, keys...).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if c == nil || c.rdb == nil {
		return false, ErrNilClient
	}
	return c.rdb.Expire(ctx, key, ttl).Result()
}

func (c *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	if c == nil || c.rdb == nil {
		return 0, ErrNilClient
	}
	return c.rdb.TTL(ctx, key).Result()
}

func (c *Client) SetNX(ctx context.Context, key string, value any, ttl time.Duration) (bool, error) {
	if c == nil || c.rdb == nil {
		return false, ErrNilClient
	}
	return c.rdb.SetNX(ctx, key, value, ttl).Result()
}

func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, ErrNilClient
	}
	return c.rdb.Incr(ctx, key).Result()
}

func (c *Client) IncrWithExpiry(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, ErrNilClient
	}
	ttlMs := ttl.Milliseconds()
	res, err := c.rdb.Eval(ctx, incrWithExpiryLuaScript, []string{key}, ttlMs).Result()
	if err != nil {
		return 0, err
	}
	if val, ok := res.(int64); ok {
		return val, nil
	}
	return 0, fmt.Errorf("unexpected return type from incrWithExpiry: %T", res)
}

func (c *Client) ReleaseLock(ctx context.Context, key, lockValue string) (bool, error) {
	if c == nil || c.rdb == nil {
		return false, ErrNilClient
	}
	res, err := c.rdb.Eval(ctx, releaseLockLuaScript, []string{key}, lockValue).Result()
	if err != nil {
		return false, err
	}
	deletedCount, ok := res.(int64)
	if !ok {
		return false, fmt.Errorf("unexpected return type from releaseLock: %T", res)
	}
	return deletedCount > 0, nil
}
