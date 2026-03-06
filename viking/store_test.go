package viking

import (
	"testing"
	"time"
)

func TestKVStore_SetGet(t *testing.T) {
	s := NewKVStore()
	s.Set("k1", "v1", LevelPublic, 0)
	v, ok := s.Get("k1", LevelPublic)
	if !ok || v != "v1" {
		t.Errorf("Get: want v1, got %s ok=%v", v, ok)
	}
}

func TestKVStore_TTL(t *testing.T) {
	s := NewKVStore()
	s.Set("k1", "v1", LevelPublic, 10*time.Millisecond)
	time.Sleep(15 * time.Millisecond)
	_, ok := s.Get("k1", LevelPublic)
	if ok {
		t.Error("expired key should not be returned")
	}
}
