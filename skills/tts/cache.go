// Package tts provides cache and concurrency control for TTS synthesis.
package tts

import (
	"sync"
	"time"
)

const (
	defaultTTSTTL       = 1 * time.Hour
	defaultTTSConcurrent = 2
)

type ttsCacheEntry struct {
	data    []byte
	expires time.Time
}

type ttsCache struct {
	mu      sync.RWMutex
	entries map[string]*ttsCacheEntry
	ttl     time.Duration
	maxSize int
}

var ttsGlobalCache *ttsCache
var ttsSem chan struct{}

func initTTSCache() {
	if ttsGlobalCache != nil {
		return
	}
	ttsGlobalCache = &ttsCache{
		entries: make(map[string]*ttsCacheEntry),
		ttl:     defaultTTSTTL,
		maxSize: 200,
	}
}

func initTTSSem() {
	if ttsSem != nil {
		return
	}
	ttsSem = make(chan struct{}, defaultTTSConcurrent)
}

func ttsCacheKey(text, voice string) string {
	return "tts:" + voice + ":" + text
}

func ttsCacheGet(key string) ([]byte, bool) {
	initTTSCache()
	ttsGlobalCache.mu.RLock()
	e := ttsGlobalCache.entries[key]
	ttsGlobalCache.mu.RUnlock()
	if e == nil || time.Now().After(e.expires) {
		return nil, false
	}
	return e.data, true
}

func ttsCacheSet(key string, data []byte) {
	initTTSCache()
	ttsGlobalCache.mu.Lock()
	defer ttsGlobalCache.mu.Unlock()
	if len(ttsGlobalCache.entries) >= ttsGlobalCache.maxSize {
		now := time.Now()
		for k, v := range ttsGlobalCache.entries {
			if now.After(v.expires) {
				delete(ttsGlobalCache.entries, k)
			}
		}
		if len(ttsGlobalCache.entries) >= ttsGlobalCache.maxSize {
			var oldest string
			var oldestT time.Time
			for k, v := range ttsGlobalCache.entries {
				if oldest == "" || v.expires.Before(oldestT) {
					oldest, oldestT = k, v.expires
				}
			}
			if oldest != "" {
				delete(ttsGlobalCache.entries, oldest)
			}
		}
	}
	ttsGlobalCache.entries[key] = &ttsCacheEntry{data: data, expires: time.Now().Add(ttsGlobalCache.ttl)}
}

func ttsAcquire() bool {
	initTTSSem()
	select {
	case ttsSem <- struct{}{}:
		return true
	default:
		return false
	}
}

func ttsRelease() {
	select {
	case <-ttsSem:
	default:
	}
}
