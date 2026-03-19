package viking

import (
	"testing"
)

func TestSearchIndex_IndexSearch(t *testing.T) {
	idx := NewSearchIndex()
	idx.Index("doc1", "hello world", nil)
	idx.Index("doc2", "world peace", nil)
	results := idx.Search("world")
	if len(results) != 2 {
		t.Errorf("want 2 results, got %d", len(results))
	}
}

func TestSearchIndex_Chinese(t *testing.T) {
	idx := NewSearchIndex()
	idx.Index("doc1", "你好世界", nil)
	results := idx.Search("你好")
	if len(results) != 1 {
		t.Errorf("want 1 result for 你好, got %d", len(results))
	}
}
