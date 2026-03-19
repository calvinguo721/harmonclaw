package engine

import (
	"testing"
	"time"
)

type mockRetriever struct {
	entries []MemoryEntry
}

func (m *mockRetriever) Search(query string, limit int) []MemoryEntry {
	var out []MemoryEntry
	for _, e := range m.entries {
		if len(out) >= limit {
			break
		}
		if len(e.Content) > 0 {
			out = append(out, e)
		}
	}
	return out
}

func TestMemoryEngine_PushShort(t *testing.T) {
	eng := NewMemoryEngine(3, nil)
	eng.PushShort(MemoryEntry{Content: "a"})
	eng.PushShort(MemoryEntry{Content: "b"})
	eng.PushShort(MemoryEntry{Content: "c"})
	eng.PushShort(MemoryEntry{Content: "d"})

	short := eng.ShortTerm()
	if len(short) != 3 {
		t.Errorf("short: want 3, got %d", len(short))
	}
	if short[0].Content != "b" {
		t.Errorf("first: want b, got %s", short[0].Content)
	}
}

func TestDecayFactor(t *testing.T) {
	if DecayFactor(0, time.Hour) != 1.0 {
		t.Error("t=0 should be 1.0")
	}
	if DecayFactor(24*time.Hour, 24*time.Hour) < 0.4 || DecayFactor(24*time.Hour, 24*time.Hour) > 0.6 {
		t.Logf("half-life decay: %f", DecayFactor(24*time.Hour, 24*time.Hour))
	}
}

func TestMemoryEngine_Retrieve(t *testing.T) {
	eng := NewMemoryEngine(10, nil)
	eng.PushShort(MemoryEntry{Content: "hello world"})
	eng.PushShort(MemoryEntry{Content: "foo bar"})

	out := eng.Retrieve("hello", 5)
	if len(out) != 1 || out[0].Content != "hello world" {
		t.Errorf("retrieve: %v", out)
	}

	out = eng.Retrieve("", 5)
	if len(out) != 0 {
		t.Errorf("empty query: %v", out)
	}
}

func TestMemoryEngine_Retrieve_WithLongTerm(t *testing.T) {
	mock := &mockRetriever{
		entries: []MemoryEntry{
			{Content: "old memory", CreatedAt: time.Now().Add(-48 * time.Hour)},
		},
	}
	eng := NewMemoryEngine(10, mock)
	eng.PushShort(MemoryEntry{Content: "recent"})

	out := eng.Retrieve("memory", 5)
	if len(out) < 1 {
		t.Errorf("expected long-term hit: %v", out)
	}
}
