// Package viking (store) provides Put/Get/Delete/List(prefix), TTL cleanup, classification.
package viking

import (
	"sort"
	"strings"
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
	Value     string
	Level     StoreLevel
	ExpiresAt time.Time
	CreatedAt time.Time
}

// KVStore is an in-memory key-value store with TTL and List(prefix).
type KVStore struct {
	mu    sync.RWMutex
	items map[string]StoreItem
}

// NewKVStore creates a store.
func NewKVStore() *KVStore {
	k := &KVStore{items: make(map[string]StoreItem)}
	go k.ttlCleanupLoop()
	return k
}

func (s *KVStore) ttlCleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.Expire()
	}
}

// Put stores a value. ttl=0 means no expiry.
func (s *KVStore) Put(key, value string, level StoreLevel, ttl time.Duration) {
	s.Set(key, value, level, ttl)
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

// List returns keys with prefix, sorted.
func (s *KVStore) List(prefix string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for k, v := range s.items {
		if prefix != "" && !strings.HasPrefix(k, prefix) {
			continue
		}
		if !v.ExpiresAt.IsZero() && time.Now().After(v.ExpiresAt) {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
