package viking

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEngram_SerializeParse(t *testing.T) {
	e := Engram{
		ID:           "e1",
		SessionID:    "s1",
		Timestamp:    time.Now(),
		Content:      "test content",
		Summary:      "summary",
		Keywords:     []string{"a", "b"},
		Entities:     []string{"x"},
		Importance:   4,
		LastAccessed: time.Now(),
		AccessCount:  1,
		Metadata:     map[string]string{"k": "v"},
	}
	data := serializeEngram(e)
	e2, err := parseEngram(data)
	if err != nil {
		t.Fatal(err)
	}
	if e2.ID != e.ID {
		t.Errorf("ID %q != %q", e2.ID, e.ID)
	}
	if e2.Summary != e.Summary {
		t.Errorf("Summary %q != %q", e2.Summary, e.Summary)
	}
	if len(e2.Keywords) != 2 {
		t.Errorf("Keywords len=%d", len(e2.Keywords))
	}
	if e2.Metadata["k"] != "v" {
		t.Errorf("Metadata k=%q", e2.Metadata["k"])
	}
}

func TestEngramStore_WriteGet(t *testing.T) {
	dir := t.TempDir()
	store := NewEngramStore(dir)
	e := Engram{
		ID:         "eng1",
		SessionID:  "sess1",
		Content:    "content",
		Summary:    "summary",
		Importance: 3,
	}
	if err := store.Write(e); err != nil {
		t.Fatal(err)
	}
	got, ok := store.Get("eng1")
	if !ok {
		t.Fatal("expected engram")
	}
	if got.Summary != "summary" {
		t.Errorf("Summary=%q", got.Summary)
	}
	fpath := filepath.Join(dir, got.Timestamp.Format("2006-01-02"), "sess1", "eng1.txt")
	if _, err := os.Stat(fpath); err != nil {
		t.Errorf("file not found: %v", err)
	}
}

func TestEngramStore_Load(t *testing.T) {
	dir := t.TempDir()
	store := NewEngramStore(dir)
	store.Write(Engram{ID: "l1", SessionID: "s1", Content: "x", Summary: "y"})
	store2 := NewEngramStore(dir)
	if err := store2.Load(); err != nil {
		t.Fatal(err)
	}
	got, ok := store2.Get("l1")
	if !ok {
		t.Fatal("expected loaded engram")
	}
	if got.Summary != "y" {
		t.Errorf("Summary=%q", got.Summary)
	}
}

func TestEngramStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store := NewEngramStore(dir)
	store.Write(Engram{ID: "d1", SessionID: "s1", Content: "x"})
	if err := store.Delete("d1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get("d1"); ok {
		t.Error("expected deleted")
	}
}

func TestEngramStore_GC(t *testing.T) {
	dir := t.TempDir()
	store := NewEngramStore(dir)
	store.maxSize = 5
	for i := 0; i < 10; i++ {
		store.Write(Engram{
			ID:           string(rune('a' + i)),
			SessionID:    "s1",
			Importance:   1,
			LastAccessed: time.Now().Add(-time.Duration(i) * 24 * time.Hour),
		})
	}
	if len(store.List()) > 10 {
		t.Errorf("GC should have evicted, got %d", len(store.List()))
	}
}
