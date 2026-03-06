package viking

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotManager_Save(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-snapshot-test")
	defer os.RemoveAll(dir)
	m := NewSnapshotManager(dir, "test", 3)
	path, err := m.Save([]byte(`{"x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Error("path should not be empty")
	}
	latest, err := m.Latest()
	if err != nil || latest != path {
		t.Errorf("Latest: want %s, got %s err=%v", path, latest, err)
	}
}
