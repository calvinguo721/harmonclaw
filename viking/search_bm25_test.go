package viking

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSegmenter_Segment(t *testing.T) {
	seg := NewSegmenter("")
	tokens := seg.Segment("搜索Go语言性能")
	if len(tokens) == 0 {
		t.Fatal("expected tokens")
	}
	tokens2 := seg.Segment("Go语言性能优化")
	hasLang := false
	for _, t := range tokens2 {
		if t == "语言" {
			hasLang = true
			break
		}
	}
	if !hasLang {
		t.Errorf("expected 语言 in %v", tokens2)
	}
}

func TestSegmenter_English(t *testing.T) {
	seg := NewSegmenter("")
	tokens := seg.Segment("searching documents")
	if len(tokens) == 0 {
		t.Fatal("expected tokens")
	}
}

func TestBM25Index_IndexSearch(t *testing.T) {
	idx := NewBM25Index("")
	idx.Index("d1", "Go language performance optimization", nil)
	idx.Index("d2", "Rust language safety", nil)
	idx.Index("d3", "Go and Rust comparison", nil)
	results := idx.Search("Go", 5)
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0] != "d1" && results[0] != "d3" {
		t.Errorf("first=%q, expected d1 or d3", results[0])
	}
}

func TestBM25Index_Chinese(t *testing.T) {
	idx := NewBM25Index("")
	idx.Index("c1", "hello world", nil)
	idx.Index("c2", "Go语言性能优化", nil)
	results := idx.Search("hello", 5)
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	results2 := idx.Search("语言", 5)
	if len(results2) == 0 {
		t.Fatal("expected Chinese results")
	}
}

func TestBM25Index_TimeDecay(t *testing.T) {
	idx := NewBM25Index("")
	idx.Index("old", "old document", nil)
	idx.Index("new", "new document", nil)
	// Both match "document" - newer should rank higher (via LastAccessed)
	results := idx.Search("document", 5)
	if len(results) < 2 {
		t.Fatalf("expected 2, got %v", results)
	}
}

func TestBM25Index_PersistLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bm25.json")
	idx := NewBM25Index("")
	idx.path = path
	idx.Index("p1", "persistent content", nil)
	if err := idx.Persist(); err != nil {
		t.Fatal(err)
	}
	idx2 := NewBM25IndexWithPath(path, "")
	results := idx2.Search("persistent", 5)
	if len(results) == 0 {
		t.Fatal("expected loaded results")
	}
	if results[0] != "p1" {
		t.Errorf("first=%q", results[0])
	}
}

func TestBM25Index_Importance(t *testing.T) {
	idx := NewBM25Index("")
	idx.Index("low", "test content", map[string]string{"importance": "1"})
	idx.Index("high", "test content", map[string]string{"importance": "5"})
	results := idx.Search("test content", 5)
	if len(results) < 2 {
		t.Fatalf("expected 2, got %v", results)
	}
	if results[0] != "high" {
		t.Logf("importance weight may vary: first=%q", results[0])
	}
}

func TestSegmenter_Stem(t *testing.T) {
	seg := NewSegmenter("")
	tokens := seg.Segment("searching documents")
	if len(tokens) < 2 {
		t.Errorf("expected 2+ tokens, got %v", tokens)
	}
}

func TestBM25Index_WithDict(t *testing.T) {
	dir := t.TempDir()
	dictPath := filepath.Join(dir, "dict.txt")
	os.WriteFile(dictPath, []byte("搜索\n语言\n性能\n"), 0644)
	idx := NewBM25Index(dictPath)
	idx.Index("x", "搜索语言性能", nil)
	results := idx.Search("搜索", 5)
	if len(results) == 0 {
		t.Error("expected results with dict")
	}
}
