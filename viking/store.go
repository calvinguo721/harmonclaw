// Package viking (store) provides KV store with TTL and access control.
package viking

import (
	"sync"
	"time"
)

// StoreLevel for access control.
type StoreLevel int

const (
	LevelPublic StoreLevel = iota
	LevelInternal
	LevelSensitive
	LevelSecret
)

// StoreItem holds value with metadata.
type StoreItem struct {
	Value      string
	Level      StoreLevel
	ExpiresAt  time.Time
	CreatedAt  time.Time
}

// KVStore is an in-memory key-value store with TTL.
type KVStore struct {
	mu    sync.RWMutex
	items map[string]StoreItem
}

// NewKVStore creates a store.
func NewKVStore() *KVStore {
	return &KVStore{items: make(map[string]StoreItem)}
}

// Set stores a value. ttl=0 means no expiry.
func (s *KVStore) Set(key, value string, level StoreLevel, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	item := StoreItem{
		Value:     value,
		Level:     level,
		CreatedAt: now,
	}
	if ttl > 0 {
		item.ExpiresAt = now.Add(ttl)
	}
	s.items[key] = item
}

// Get returns value if not expired and caller has access.
func (s *KVStore) Get(key string, minLevel StoreLevel) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[key]
	if !ok {
		return "", false
	}
	if !item.ExpiresAt.IsZero() && time.Now().After(item.ExpiresAt) {
		delete(s.items, key)
		return "", false
	}
	if minLevel > 0 && item.Level < minLevel {
		return "", false
	}
	return item.Value, true
}

// Delete removes a key.
func (s *KVStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
}

// Expire cleans expired entries.
func (s *KVStore) Expire() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	n := 0
	for k, v := range s.items {
		if !v.ExpiresAt.IsZero() && now.After(v.ExpiresAt) {
			delete(s.items, k)
			n++
		}
	}
	return n
}

// Len returns number of keys.
func (s *KVStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}
