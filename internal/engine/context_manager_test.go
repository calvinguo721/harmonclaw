package engine

import (
	"strings"
	"testing"
	"time"
)

type mockWriter struct {
	written []string
}

func (m *mockWriter) WriteEngram(sessionID, summary string) error {
	m.written = append(m.written, summary)
	return nil
}

type mockRetriever struct {
	results []string
}

func (m *mockRetriever) Search(query string, limit int) []string {
	return m.results
}

func TestContextManager_Append(t *testing.T) {
	cm := NewContextManager(nil, nil)
	turns, err := cm.Append("s1", "user", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Content != "hello" {
		t.Errorf("append: %v", turns)
	}
}

func TestContextManager_SlidingWindow(t *testing.T) {
	w := &mockWriter{}
	cm := NewContextManager(w, nil)
	cm.windowSize = 6

	for i := 0; i < 5; i++ {
		cm.Append("s1", "user", "msg")
		cm.Append("s1", "assistant", "reply")
	}

	if len(w.written) == 0 {
		t.Error("expected summary write")
	}
}

func TestContextManager_GetRecent(t *testing.T) {
	cm := NewContextManager(nil, nil)
	cm.Append("s1", "user", "a")
	cm.Append("s1", "assistant", "b")
	cm.Append("s1", "user", "c")

	recent := cm.GetRecent("s1", 2)
	if len(recent) != 2 || recent[0].Content != "b" {
		t.Errorf("get recent: %v", recent)
	}
}

func TestContextManager_GetContextForLLM(t *testing.T) {
	r := &mockRetriever{results: []string{"mem1"}}
	cm := NewContextManager(nil, r)
	cm.Append("s1", "user", "hi")
	cm.Append("s1", "assistant", "hello")

	turns, mems := cm.GetContextForLLM("s1", "test")
	if len(turns) != 2 || len(mems) != 1 || mems[0] != "mem1" {
		t.Errorf("context: turns=%d mems=%v", len(turns), mems)
	}
}

func TestContextManager_ArchiveStale(t *testing.T) {
	cm := NewContextManager(nil, nil)
	cm.SessionTimeout = 1 * time.Millisecond
	cm.Append("s1", "user", "hi")

	time.Sleep(5 * time.Millisecond)

	n := cm.ArchiveStale()
	if n != 1 {
		t.Errorf("archive: want 1, got %d", n)
	}
	if cm.SessionCount() != 0 {
		t.Error("session should be removed")
	}
}

func TestFormatContextForPrompt(t *testing.T) {
	turns := []Turn{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}}
	memories := []string{"mem1"}
	s := FormatContextForPrompt(turns, memories)
	if !contains(s, "mem1") || !contains(s, "user: hi") {
		t.Errorf("format: %s", s)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
