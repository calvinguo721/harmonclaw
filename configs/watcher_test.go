package configs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcher_Changed(t *testing.T) {
	dir := t.TempDir()
	w := NewWatcher(dir)

	w.seedModTimes()
	if w.changed() {
		t.Error("empty dir should not trigger change")
	}

	f := filepath.Join(dir, "test.json")
	os.WriteFile(f, []byte("{}"), 0644)
	w.seedModTimes()

	w.mu.Lock()
	w.modTimes[f] = time.Now().Add(-time.Hour)
	w.mu.Unlock()

	if !w.changed() {
		t.Error("modified file should trigger change")
	}
}

func TestWatcher_StartStop(t *testing.T) {
	dir := t.TempDir()
	w := NewWatcher(dir)
	done := make(chan struct{})
	go func() {
		w.Start()
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	w.Stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Stop did not terminate Start")
	}
}
