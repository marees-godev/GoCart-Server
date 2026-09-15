package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool   *pgxpool.Pool
	Config Config
}

func New(ctx context.Context, cfg Config) (*DB, error) {
	if cfg.URL == "" {
		return nil, errors.New("database URL cannot be empty")
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database configuration: %w", sanitizeURLError(err, cfg.URL))
	}

	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns >= 0 {
		poolCfg.MinConns = cfg.MinConns
	}
	if cfg.MaxConnLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	}
	if cfg.MaxConnIdleTime > 0 {
		poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	}
	if cfg.HealthCheckPeriod > 0 {
		poolCfg.HealthCheckPeriod = cfg.HealthCheckPeriod
	}

	maxAttempts := cfg.MaxRetries
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	retryDelay := cfg.RetryInterval
	if retryDelay <= 0 {
		retryDelay = 1 * time.Second
	}

	var pool *pgxpool.Pool
	var lastErr error
	sanitizedURL := SanitizeURL(cfg.URL)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		connCtx := ctx
		var cancel context.CancelFunc
		if cfg.ConnectTimeout > 0 {
			connCtx, cancel = context.WithTimeout(ctx, cfg.ConnectTimeout)
		}

		pool, lastErr = pgxpool.NewWithConfig(connCtx, poolCfg)
		if cancel != nil {
			cancel()
		}

		if lastErr == nil {
			pingTimeout := 15 * time.Second
			if cfg.ConnectTimeout > 0 {
				pingTimeout = cfg.ConnectTimeout
			}
			pingCtx, pingCancel := context.WithTimeout(ctx, pingTimeout)
			lastErr = pool.Ping(pingCtx)
			pingCancel()

			if lastErr == nil {
				slog.Info("Connected to database",
					"max_conns", poolCfg.MaxConns,
					"min_conns", poolCfg.MinConns,
					"target", sanitizedURL,
				)
				return &DB{Pool: pool, Config: cfg}, nil
			}

			pool.Close()
		}

		slog.Warn("Database connection attempt failed",
			"attempt", attempt,
			"error", lastErr.Error(),
			"retry_in", retryDelay,
		)

		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during database retry: %w", ctx.Err())
			case <-time.After(retryDelay):
				retryDelay *= 2
			}
		}
	}

	return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxAttempts, lastErr)
}

func (db *DB) Ping(ctx context.Context) error {
	if db.Pool == nil {
		return errors.New("database pool is not initialized")
	}
	return db.Pool.Ping(ctx)
}

func (db *DB) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}

func (db *DB) Stats() *pgxpool.Stat {
	if db.Pool == nil {
		return nil
	}
	return db.Pool.Stat()
}

func (db *DB) WithTransaction(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx error: %w, rollback error: %v", err, rbErr)
		}
		return err
	}

	return tx.Commit(ctx)
}

func (db *DB) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return db.Pool.Exec(ctx, sql, arguments...)
}

func (db *DB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return db.Pool.Query(ctx, sql, args...)
}

func (db *DB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return db.Pool.QueryRow(ctx, sql, args...)
}

func SanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "[malformed database url]"
	}
	return u.Redacted()
}

func sanitizeURLError(err error, rawURL string) error {
	if err == nil {
		return nil
	}
	sanitized := SanitizeURL(rawURL)
	return errors.New(strings.ReplaceAll(err.Error(), rawURL, sanitized))
}
