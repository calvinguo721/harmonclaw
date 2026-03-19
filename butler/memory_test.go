package butler

import (
	"os"
	"path/filepath"
	"testing"

	"harmonclaw/viking"
)

func TestMemoryManager_Append(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-mem-test")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	store, _ := viking.NewFileStore(dir)
	cm := NewConversationManager(store, dir)
	mm := NewMemoryManager(cm, dir)
	msgs, err := mm.Append("u1", "s1", "user", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Errorf("Append: want 1, got %d", len(msgs))
	}
	msgs, _ = mm.Append("u1", "s1", "assistant", "hi")
	if len(msgs) != 2 {
		t.Errorf("Append: want 2, got %d", len(msgs))
	}
}

func TestMemoryManager_InjectSummary(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-mem-summary")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	store, _ := viking.NewFileStore(dir)
	cm := NewConversationManager(store, dir)
	mm := NewMemoryManager(cm, dir)
	for i := 0; i < 12; i++ {
		mm.Append("u2", "s2", "user", "q")
		mm.Append("u2", "s2", "assistant", "a")
	}
	ctx := mm.GetContext("u2", "s2")
	if len(ctx) == 0 {
		t.Error("GetContext should return messages")
	}
}
