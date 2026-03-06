// Package governor (ratelimit) provides token bucket rate limiting.
package governor

import (
	"sync"
	"time"
)

// TokenBucket implements token bucket rate limiting with burst support.
type TokenBucket struct {
	mu sync.Mutex

	capacity  float64
	tokens    float64
	refill    float64 // tokens per second
	lastRefill time.Time
}

// NewTokenBucket creates a bucket. rate = tokens/sec, burst = max tokens.
func NewTokenBucket(rate float64, burst int) *TokenBucket {
	if rate <= 0 {
		rate = 1
	}
	cap := float64(burst)
	if cap < 1 {
		cap = 1
	}
	return &TokenBucket{
		capacity:  cap,
		tokens:    cap,
		refill:    rate,
		lastRefill: time.Now(),
	}
}

// Allow consumes one token if available. Returns true if allowed.
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refill
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
	tb.lastRefill = now
	if tb.tokens >= 1 {
		tb.tokens -= 1
		return true
	}
	return false
}

// AllowN consumes n tokens if available.
func (tb *TokenBucket) AllowN(n int) bool {
	if n <= 0 {
		return true
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refill
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
	tb.lastRefill = now
	fn := float64(n)
	if tb.tokens >= fn {
		tb.tokens -= fn
		return true
	}
	return false
}

// RateLimitMap manages per-key token buckets.
type RateLimitMap struct {
	mu      sync.Mutex
	buckets map[string]*TokenBucket
	rate    float64
	burst   int
}

// NewRateLimitMap creates a map with default rate and burst per key.
func NewRateLimitMap(rate float64, burst int) *RateLimitMap {
	return &RateLimitMap{
		buckets: make(map[string]*TokenBucket),
		rate:    rate,
		burst:   burst,
	}
}

// Allow consumes one token for key.
func (m *RateLimitMap) Allow(key string) bool {
	m.mu.Lock()
	b, ok := m.buckets[key]
	if !ok {
		b = NewTokenBucket(m.rate, m.burst)
		m.buckets[key] = b
	}
	m.mu.Unlock()
	return b.Allow()
}
