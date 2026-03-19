// Package web_search provides in-memory cache for search results.
package web_search

import (
	"sync"
	"time"
)

const (
	defaultCacheTTL       = 10 * time.Minute
	defaultMaxCacheEntries = 500
	defaultMaxConcurrent   = 3
)

type cacheEntry struct {
	data    []byte
	expires time.Time
}

type searchCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
	maxSize int
}

var globalCache *searchCache
var searchSem chan struct{}

func initCache() {
	if globalCache != nil {
		return
	}
	globalCache = &searchCache{
		entries: make(map[string]*cacheEntry),
		ttl:     defaultCacheTTL,
		maxSize: defaultMaxCacheEntries,
	}
}

func initSemaphore() {
	if searchSem != nil {
		return
	}
	n := defaultMaxConcurrent
	searchSem = make(chan struct{}, n)
}

func cacheGet(key string) ([]byte, bool) {
	initCache()
	globalCache.mu.RLock()
	e := globalCache.entries[key]
	globalCache.mu.RUnlock()
	if e == nil || time.Now().After(e.expires) {
		return nil, false
	}
	return e.data, true
}

func cacheSet(key string, data []byte) {
	initCache()
	globalCache.mu.Lock()
	defer globalCache.mu.Unlock()
	if len(globalCache.entries) >= globalCache.maxSize {
		now := time.Now()
		for k, v := range globalCache.entries {
			if now.After(v.expires) {
				delete(globalCache.entries, k)
			}
		}
		if len(globalCache.entries) >= globalCache.maxSize {
			var oldest string
			var oldestT time.Time
			for k, v := range globalCache.entries {
				if oldest == "" || v.expires.Before(oldestT) {
					oldest, oldestT = k, v.expires
				}
			}
			if oldest != "" {
				delete(globalCache.entries, oldest)
			}
		}
	}
	globalCache.entries[key] = &cacheEntry{data: data, expires: time.Now().Add(globalCache.ttl)}
}

func acquireSearchSlot() bool {
	initSemaphore()
	select {
	case searchSem <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseSearchSlot() {
	select {
	case <-searchSem:
	default:
	}
}
