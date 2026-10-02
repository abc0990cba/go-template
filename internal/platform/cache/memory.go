// Package cache provides the Cache port adapters: an in-process TTL map for
// development and single-replica mode, and Redis for shared deployments.
package cache

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Memory is an in-process TTL cache; safe for concurrent use and good
// enough for the single-process development mode.
type Memory struct {
	mu      sync.RWMutex
	entries map[string]memoryEntry
}

type memoryEntry struct {
	value   []byte
	expires time.Time
}

// NewMemory builds the in-process cache.
func NewMemory() *Memory {
	return &Memory{entries: map[string]memoryEntry{}}
}

// Get returns the cached value for the key, if present and fresh.
func (m *Memory) Get(_ context.Context, key string) ([]byte, bool) {
	m.mu.RLock()
	entry, ok := m.entries[key]
	m.mu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.value, true
}

// Set stores the value under the key until the TTL expires.
func (m *Memory) Set(_ context.Context, key string, value []byte, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = memoryEntry{value: value, expires: time.Now().Add(ttl)}
}

// Delete drops the keys from the cache.
func (m *Memory) Delete(_ context.Context, keys ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		delete(m.entries, key)
	}
}

// DeletePrefix drops every key starting with the prefix.
func (m *Memory) DeletePrefix(_ context.Context, prefix string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.entries {
		if strings.HasPrefix(key, prefix) {
			delete(m.entries, key)
		}
	}
}
