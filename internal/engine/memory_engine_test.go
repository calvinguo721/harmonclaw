package engine

import (
	"strings"
	"testing"
	"time"
)

func TestMemoryEngine_AppendShortTerm(t *testing.T) {
	store := NewMemStore()
	me := NewMemoryEngine(store)
	me.AppendShortTerm("s1", "user", "hello")
	me.AppendShortTerm("s1", "assistant", "hi there")
	turns := me.getShortTerm("s1")
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	if turns[0].Content != "hello" {
		t.Errorf("turn0=%q", turns[0].Content)
	}
}

func TestMemoryEngine_FlushSession(t *testing.T) {
	store := NewMemStore()
	me := NewMemoryEngine(store)
	me.AppendShortTerm("s2", "user", "我的名字是张三")
	me.AppendShortTerm("s2", "assistant", "你好张三")
	me.FlushSession("s2")
	turns := me.getShortTerm("s2")
	if len(turns) != 0 {
		t.Errorf("short-term should be cleared, got %d", len(turns))
	}
}

func TestMemoryEngine_WriteEngram(t *testing.T) {
	store := NewMemStore()
	me := NewMemoryEngine(store)
	err := me.WriteEngram("s3", "用户提到了项目截止日期 2024-03-15")
	if err != nil {
		t.Fatal(err)
	}
	results := me.Search("截止日期", 5)
	if len(results) == 0 {
		t.Error("expected search result")
	}
}

func TestMemoryEngine_Search(t *testing.T) {
	store := NewMemStore()
	me := NewMemoryEngine(store)
	me.WriteEngram("s4", "讨论 Go 语言性能优化")
	me.WriteEngram("s4", "讨论 Rust 语言")
	results := me.Search("Go", 5)
	if len(results) == 0 {
		t.Fatal("expected Go result")
	}
	found := false
	for _, r := range results {
		if strings.Contains(r, "Go") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("results=%v", results)
	}
}

func TestExtractKeywords(t *testing.T) {
	kw := extractKeywords("Go 语言 性能 优化 很好")
	if len(kw) == 0 {
		t.Error("expected keywords")
	}
}

func TestExtractEntities(t *testing.T) {
	ent := extractEntities("会议在 2024-03-15 举行，预算 100 万")
	if len(ent) < 2 {
		t.Errorf("expected entities, got %v", ent)
	}
}

func TestComputeImportance(t *testing.T) {
	if computeImportance([]string{"a", "b", "c"}, []string{"x"}) < 2 {
		t.Error("expected importance >= 2")
	}
}

func TestJaccard(t *testing.T) {
	a := []string{"a", "b", "c"}
	b := []string{"a", "b", "d"}
	j := jaccard(a, b)
	if j <= 0 || j > 1 {
		t.Errorf("jaccard=%f", j)
	}
}

func TestMergeSimilar(t *testing.T) {
	store := NewMemStore()
	e1 := Engram{ID: "1", Summary: "go lang", Keywords: []string{"go", "lang"}, Timestamp: time.Now()}
	e2 := Engram{ID: "2", Summary: "go language", Keywords: []string{"go", "language"}, Timestamp: time.Now()}
	store.Write(e1)
	store.Write(e2)
	me := NewMemoryEngine(store)
	results := me.Search("go", 5)
	if len(results) == 0 {
		t.Error("expected results")
	}
}

func TestMemStore_UpdateAccess(t *testing.T) {
	store := NewMemStore()
	e := Engram{ID: "acc", Summary: "test", LastAccessed: time.Now().Add(-24 * time.Hour)}
	store.Write(e)
	store.UpdateAccess("acc", time.Now())
	// just ensure no panic
}
