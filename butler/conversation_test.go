package butler

import (
	"os"
	"path/filepath"
	"testing"

	"harmonclaw/llm"
	"harmonclaw/viking"
)

func TestConversationManager_Append_SlidingWindow(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-conv-test")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	store, _ := viking.NewFileStore(dir)
	cm := NewConversationManager(store, dir)
	cm.SetContextWindow(5)
	for i := 0; i < 8; i++ {
		msgs, err := cm.Append("user1", "s1", "user", "msg")
		if err != nil {
			t.Fatal(err)
		}
		msgs, _ = cm.Append("user1", "s1", "assistant", "reply")
		if len(msgs) > 5 {
			t.Errorf("window 5: got %d messages", len(msgs))
		}
	}
	ctx := cm.GetContext("user1", "s1")
	if len(ctx) > 5 {
		t.Errorf("GetContext: want <=5, got %d", len(ctx))
	}
}

func TestConversationManager_LoadContext(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-conv-load")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	store, _ := viking.NewFileStore(dir)
	cm := NewConversationManager(store, dir)
	cm.Append("u2", "s2", "user", "hello")
	cm.Append("u2", "s2", "assistant", "hi")
	msgs, err := cm.LoadContext("u2", "s2")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Errorf("LoadContext: want 2, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("LoadContext: wrong content %v", msgs[0])
	}
}

func TestConversationManager_MessageTypes(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-conv-types")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	store, _ := viking.NewFileStore(dir)
	cm := NewConversationManager(store, dir)
	cm.Append("u3", "s3", "user", "q")
	msgs, _ := cm.Append("u3", "s3", "assistant", "a")
	_ = msgs
	var m llm.Message
	m.Role = "user"
	m.Content = "test"
	if m.Role == "" {
		t.Error("Message role empty")
	}
}
