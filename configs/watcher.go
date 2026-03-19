// Package configs provides config file watcher with hot-reload.
package configs

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"harmonclaw/bus"
)

const pollInterval = 10 * time.Second

// Watcher polls configs/ for file changes and sends bus events.
type Watcher struct {
	dir      string
	modTimes map[string]time.Time
	mu       sync.Mutex
	stop     chan struct{}
}

// NewWatcher creates a watcher for the given config directory.
func NewWatcher(dir string) *Watcher {
	if dir == "" {
		dir = "configs"
	}
	return &Watcher{
		dir:      dir,
		modTimes: make(map[string]time.Time),
		stop:     make(chan struct{}),
	}
}

// Start begins polling. Call from a goroutine.
func (w *Watcher) Start() {
	w.seedModTimes()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
		if w.changed() {
			bus.Publish(bus.EventConfigReloaded, map[string]string{"path": w.dir})
		}
		}
	}
}

// Stop stops the watcher.
func (w *Watcher) Stop() {
	close(w.stop)
}

func (w *Watcher) seedModTimes() {
	w.mu.Lock()
	defer w.mu.Unlock()
	entries, _ := os.ReadDir(w.dir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(w.dir, e.Name())
		if info, err := os.Stat(path); err == nil {
			w.modTimes[path] = info.ModTime()
		}
	}
}

func (w *Watcher) changed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(w.dir, e.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		mt := info.ModTime()
		old, ok := w.modTimes[path]
		if !ok {
			w.modTimes[path] = mt
			return true
		}
		if !mt.Equal(old) {
			w.modTimes[path] = mt
			return true
		}
	}
	return false
}
