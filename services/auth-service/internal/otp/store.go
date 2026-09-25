package otp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/redis"
)

var (
	ErrOTPNotFound = errors.New("otp not found or expired")
)

type Store interface {
	SetOTP(ctx context.Context, email string, otpHash string, ttl time.Duration) error
	GetOTP(ctx context.Context, email string) (string, error)
	DeleteOTP(ctx context.Context, email string) error
}

type redisOTPStore struct {
	client *redis.Client
}

func NewRedisStore(client *redis.Client) Store {
	return &redisOTPStore{client: client}
}

func (s *redisOTPStore) key(email string) string {
	return fmt.Sprintf("auth:otp:email:%s", email)
}

func (s *redisOTPStore) SetOTP(ctx context.Context, email string, otpHash string, ttl time.Duration) error {
	return s.client.Set(ctx, s.key(email), otpHash, ttl)
}

func (s *redisOTPStore) GetOTP(ctx context.Context, email string) (string, error) {
	val, err := s.client.Get(ctx, s.key(email))
	if err != nil {
		if errors.Is(err, redis.ErrKeyNotFound) {
			return "", ErrOTPNotFound
		}
		return "", err
	}
	return val, nil
}

func (s *redisOTPStore) DeleteOTP(ctx context.Context, email string) error {
	return s.client.Delete(ctx, s.key(email))
}

type memoryOTPStore struct {
	mu   sync.RWMutex
	data map[string]memoryOTPEntry
}

type memoryOTPEntry struct {
	hash      string
	expiresAt time.Time
}

func NewMemoryStore() Store {
	return &memoryOTPStore{
		data: make(map[string]memoryOTPEntry),
	}
}

func (s *memoryOTPStore) SetOTP(ctx context.Context, email string, otpHash string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[email] = memoryOTPEntry{
		hash:      otpHash,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (s *memoryOTPStore) GetOTP(ctx context.Context, email string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.data[email]
	if !ok || time.Now().After(entry.expiresAt) {
		return "", ErrOTPNotFound
	}
	return entry.hash, nil
}

func (s *memoryOTPStore) DeleteOTP(ctx context.Context, email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, email)
	return nil
}
