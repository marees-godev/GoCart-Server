package otp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/redis"
)

var (
	ErrResetOTPNotFound = errors.New("password reset code not found or expired")
	ErrResetLocked      = errors.New("password reset is temporarily locked due to too many failed attempts")
)

type ResetOTPData struct {
	Hash      string    `json:"hash"`
	Salt      string    `json:"salt"`
	ExpiresAt time.Time `json:"expires_at"`
	Attempts  int       `json:"attempts"`
}

type PasswordResetStore interface {
	RecordRequest(ctx context.Context, identifier string, maxRequests int, window time.Duration) (bool, error)
	SetResetOTP(ctx context.Context, email string, hash string, salt string, ttl time.Duration) error
	GetResetOTP(ctx context.Context, email string) (*ResetOTPData, error)
	IncrementAttempts(ctx context.Context, email string) (int, error)
	LockReset(ctx context.Context, email string, duration time.Duration) error
	IsLocked(ctx context.Context, email string) (bool, time.Duration, error)
	DeleteResetOTP(ctx context.Context, email string) error
}

type redisPasswordResetStore struct {
	client *redis.Client
}

func NewRedisPasswordResetStore(client *redis.Client) PasswordResetStore {
	return &redisPasswordResetStore{client: client}
}

func (s *redisPasswordResetStore) reqKey(identifier string) string {
	return fmt.Sprintf("auth:pwd_reset:req:%s", cleanEmail(identifier))
}

func (s *redisPasswordResetStore) otpKey(email string) string {
	return fmt.Sprintf("auth:pwd_reset:otp:%s", cleanEmail(email))
}

func (s *redisPasswordResetStore) attemptsKey(email string) string {
	return fmt.Sprintf("auth:pwd_reset:attempts:%s", cleanEmail(email))
}

func (s *redisPasswordResetStore) lockKey(email string) string {
	return fmt.Sprintf("auth:pwd_reset:lock:%s", cleanEmail(email))
}

func (s *redisPasswordResetStore) RecordRequest(ctx context.Context, identifier string, maxRequests int, window time.Duration) (bool, error) {
	key := s.reqKey(identifier)
	count, err := s.client.IncrWithExpiry(ctx, key, window)
	if err != nil {
		return true, err
	}
	if int(count) > maxRequests {
		return false, nil
	}
	return true, nil
}

func (s *redisPasswordResetStore) SetResetOTP(ctx context.Context, email string, hash string, salt string, ttl time.Duration) error {
	data := ResetOTPData{
		Hash:      hash,
		Salt:      salt,
		ExpiresAt: time.Now().Add(ttl),
		Attempts:  0,
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_ = s.client.Delete(ctx, s.attemptsKey(email))
	return s.client.Set(ctx, s.otpKey(email), string(payload), ttl)
}

func (s *redisPasswordResetStore) GetResetOTP(ctx context.Context, email string) (*ResetOTPData, error) {
	val, err := s.client.Get(ctx, s.otpKey(email))
	if err != nil {
		if errors.Is(err, redis.ErrKeyNotFound) {
			return nil, ErrResetOTPNotFound
		}
		return nil, err
	}
	var data ResetOTPData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, err
	}
	if time.Now().After(data.ExpiresAt) {
		_ = s.DeleteResetOTP(ctx, email)
		return nil, ErrResetOTPNotFound
	}

	// Fetch dynamic attempts if stored separately
	if attemptsStr, err := s.client.Get(ctx, s.attemptsKey(email)); err == nil {
		var a int
		if _, scanErr := fmt.Sscanf(attemptsStr, "%d", &a); scanErr == nil {
			data.Attempts = a
		}
	}
	return &data, nil
}

func (s *redisPasswordResetStore) IncrementAttempts(ctx context.Context, email string) (int, error) {
	count, err := s.client.IncrWithExpiry(ctx, s.attemptsKey(email), 1*time.Hour)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *redisPasswordResetStore) LockReset(ctx context.Context, email string, duration time.Duration) error {
	_ = s.DeleteResetOTP(ctx, email)
	return s.client.Set(ctx, s.lockKey(email), "locked", duration)
}

func (s *redisPasswordResetStore) IsLocked(ctx context.Context, email string) (bool, time.Duration, error) {
	ttl, err := s.client.TTL(ctx, s.lockKey(email))
	if err != nil {
		if errors.Is(err, redis.ErrKeyNotFound) {
			return false, 0, nil
		}
		return false, 0, err
	}
	if ttl > 0 {
		return true, ttl, nil
	}
	return false, 0, nil
}

func (s *redisPasswordResetStore) DeleteResetOTP(ctx context.Context, email string) error {
	_ = s.client.Delete(ctx, s.otpKey(email), s.attemptsKey(email))
	return nil
}

type memoryPasswordResetStore struct {
	mu           sync.RWMutex
	requests     map[string][]time.Time
	data         map[string]ResetOTPData
	attempts     map[string]int
	lockedUntil  map[string]time.Time
}

func NewMemoryPasswordResetStore() PasswordResetStore {
	return &memoryPasswordResetStore{
		requests:    make(map[string][]time.Time),
		data:        make(map[string]ResetOTPData),
		attempts:    make(map[string]int),
		lockedUntil: make(map[string]time.Time),
	}
}

func (s *memoryPasswordResetStore) RecordRequest(ctx context.Context, identifier string, maxRequests int, window time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := cleanEmail(identifier)
	now := time.Now()
	cutoff := now.Add(-window)

	var valid []time.Time
	for _, t := range s.requests[id] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= maxRequests {
		s.requests[id] = valid
		return false, nil
	}

	valid = append(valid, now)
	s.requests[id] = valid
	return true, nil
}

func (s *memoryPasswordResetStore) SetResetOTP(ctx context.Context, email string, hash string, salt string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	em := cleanEmail(email)
	s.data[em] = ResetOTPData{
		Hash:      hash,
		Salt:      salt,
		ExpiresAt: time.Now().Add(ttl),
		Attempts:  0,
	}
	delete(s.attempts, em)
	return nil
}

func (s *memoryPasswordResetStore) GetResetOTP(ctx context.Context, email string) (*ResetOTPData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	em := cleanEmail(email)
	entry, ok := s.data[em]
	if !ok || time.Now().After(entry.ExpiresAt) {
		return nil, ErrResetOTPNotFound
	}

	entry.Attempts = s.attempts[em]
	return &entry, nil
}

func (s *memoryPasswordResetStore) IncrementAttempts(ctx context.Context, email string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	em := cleanEmail(email)
	s.attempts[em]++
	return s.attempts[em], nil
}

func (s *memoryPasswordResetStore) LockReset(ctx context.Context, email string, duration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	em := cleanEmail(email)
	delete(s.data, em)
	delete(s.attempts, em)
	s.lockedUntil[em] = time.Now().Add(duration)
	return nil
}

func (s *memoryPasswordResetStore) IsLocked(ctx context.Context, email string) (bool, time.Duration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	em := cleanEmail(email)
	until, ok := s.lockedUntil[em]
	if !ok {
		return false, 0, nil
	}
	remaining := time.Until(until)
	if remaining <= 0 {
		return false, 0, nil
	}
	return true, remaining, nil
}

func (s *memoryPasswordResetStore) DeleteResetOTP(ctx context.Context, email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	em := cleanEmail(email)
	delete(s.data, em)
	delete(s.attempts, em)
	return nil
}
