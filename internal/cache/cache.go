// Package cache provides a small key-value cache used to keep hot paths
// (auth checks on every update, settings reads, explorer page listings)
// fast without hitting MongoDB each time. If REDIS_ADDR is configured it
// uses Redis (shared across restarts and, if ever scaled out, across
// instances); otherwise it falls back to an in-process map with the same
// interface so the rest of the codebase never needs to know which backend
// is active.
package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache is the interface every handler depends on.
type Cache interface {
	Get(ctx context.Context, key string, dest interface{}) (bool, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	// Invalidate removes every key sharing a prefix, e.g. clearing all
	// cached explorer pages for a user after a rename/delete.
	InvalidatePrefix(ctx context.Context, prefix string)
}

// New builds a Redis-backed cache if addr is non-empty, otherwise an
// in-memory cache.
func New(addr, password string, db int) Cache {
	if addr == "" {
		return newMemoryCache()
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db})
	return &redisCache{client: rdb}
}

// ---- Redis implementation ----

type redisCache struct {
	client *redis.Client

	// mu guards a small local prefix registry so InvalidatePrefix can use
	// SCAN without needing a Lua script; fine at the scale of one VPS bot.
	mu sync.Mutex
}

func (c *redisCache) Get(ctx context.Context, key string, dest interface{}) (bool, error) {
	raw, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(raw, dest)
}

func (c *redisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, raw, ttl).Err()
}

func (c *redisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

func (c *redisCache) InvalidatePrefix(ctx context.Context, prefix string) {
	iter := c.client.Scan(ctx, 0, prefix+"*", 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if len(keys) > 0 {
		c.client.Del(ctx, keys...)
	}
}

// ---- In-memory implementation ----

type memoryEntry struct {
	value   []byte
	expires time.Time
}

type memoryCache struct {
	mu   sync.RWMutex
	data map[string]memoryEntry
}

func newMemoryCache() *memoryCache {
	mc := &memoryCache{data: make(map[string]memoryEntry)}
	go mc.janitor()
	return mc
}

// janitor periodically sweeps expired entries so a long-running bot
// doesn't slowly leak memory from never-read cache keys.
func (m *memoryCache) janitor() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		now := time.Now()
		m.mu.Lock()
		for k, v := range m.data {
			if !v.expires.IsZero() && now.After(v.expires) {
				delete(m.data, k)
			}
		}
		m.mu.Unlock()
	}
}

func (m *memoryCache) Get(_ context.Context, key string, dest interface{}) (bool, error) {
	m.mu.RLock()
	entry, ok := m.data[key]
	m.mu.RUnlock()
	if !ok {
		return false, nil
	}
	if !entry.expires.IsZero() && time.Now().After(entry.expires) {
		m.mu.Lock()
		delete(m.data, key)
		m.mu.Unlock()
		return false, nil
	}
	return true, json.Unmarshal(entry.value, dest)
}

func (m *memoryCache) Set(_ context.Context, key string, value interface{}, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var expires time.Time
	if ttl > 0 {
		expires = time.Now().Add(ttl)
	}
	m.mu.Lock()
	m.data[key] = memoryEntry{value: raw, expires: expires}
	m.mu.Unlock()
	return nil
}

func (m *memoryCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.data, key)
	m.mu.Unlock()
	return nil
}

func (m *memoryCache) InvalidatePrefix(_ context.Context, prefix string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(m.data, k)
		}
	}
}
