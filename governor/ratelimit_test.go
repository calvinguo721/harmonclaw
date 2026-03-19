package governor

import (
	"testing"
)

func TestTokenBucket_Allow(t *testing.T) {
	tb := NewTokenBucket(10, 2)
	if !tb.Allow() {
		t.Error("first allow should succeed")
	}
	if !tb.Allow() {
		t.Error("second allow should succeed")
	}
	if tb.Allow() {
		t.Error("third should fail (empty bucket)")
	}
}

func TestRateLimitMap_Allow(t *testing.T) {
	m := NewRateLimitMap(100, 1)
	if !m.Allow("user1") {
		t.Error("first allow should succeed")
	}
	if m.Allow("user1") {
		t.Error("second should fail")
	}
	if !m.Allow("user2") {
		t.Error("different key should succeed")
	}
}
