package viking

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotManager_Snapshot(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-snapshot-test")
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("hello"), 0644)
	defer os.RemoveAll(dir)
	m := NewSnapshotManager(dir, srcDir, 3)
	path, err := m.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Error("path should not be empty")
	}
	list := m.ListSnapshots()
	if len(list) == 0 {
		t.Error("ListSnapshots should not be empty")
	}
}
