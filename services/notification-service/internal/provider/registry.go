package provider

import (
	"fmt"
	"strings"
	"sync"
)

type Registry interface {
	Register(channel string, p NotificationProvider)
	Get(channel string) (NotificationProvider, error)
	GetAll() map[string]NotificationProvider
}

type providerRegistry struct {
	mu        sync.RWMutex
	providers map[string]NotificationProvider
}

func NewRegistry() Registry {
	return &providerRegistry{
		providers: make(map[string]NotificationProvider),
	}
}

func (r *providerRegistry) Register(channel string, p NotificationProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := strings.ToUpper(strings.TrimSpace(channel))
	r.providers[key] = p
}

func (r *providerRegistry) Get(channel string) (NotificationProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := strings.ToUpper(strings.TrimSpace(channel))
	p, ok := r.providers[key]
	if !ok || p == nil {
		return nil, fmt.Errorf("%w: %s", ErrChannelNotSupported, channel)
	}
	return p, nil
}

func (r *providerRegistry) GetAll() map[string]NotificationProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make(map[string]NotificationProvider, len(r.providers))
	for k, v := range r.providers {
		res[k] = v
	}
	return res
}

func NewDefaultRegistry() Registry {
	reg := NewRegistry()
	reg.Register("EMAIL", NewMockEmailProvider(0))
	reg.Register("PUSH", NewMockPushProvider(0))
	reg.Register("IN_APP", NewInAppProvider())
	return reg
}

