// Package viking (gc) provides TTL cleanup, old snapshots, 90-day engram, hourly, Ledger.
package viking

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	gcInterval     = time.Hour
	engramMaxAge   = 90 * 24 * time.Hour
)

// GC runs TTL cleanup, removes old snapshots, removes engrams >90 days. Writes Ledger.
type GC struct {
	store    *KVStore
	snap     *SnapshotManager
	engramDir string
	ledger   Ledger
}

// NewGC creates a GC.
func NewGC(store *KVStore, snap *SnapshotManager, engramDir string, ledger Ledger) *GC {
	return &GC{
		store:    store,
		snap:     snap,
		engramDir: engramDir,
		ledger:   ledger,
	}
}

// Run performs one GC cycle.
func (g *GC) Run() {
	if g.store != nil {
		n := g.store.Expire()
		g.record("ttl_cleanup", n)
	}
	if g.snap != nil {
		list := g.snap.ListSnapshots()
		if len(list) > 24 {
			for _, p := range list[24:] {
				os.Remove(p)
			}
			g.record("snapshot_trim", len(list)-24)
		}
	}
	if g.engramDir != "" {
		n := g.cleanupEngrams()
		g.record("engram_cleanup", n)
	}
}

func (g *GC) record(action string, count int) {
	if g.ledger == nil {
		return
	}
	g.ledger.Record(LedgerEntry{
		OperatorID: "viking",
		ActionType: "gc_" + action,
		Resource:   fmt.Sprintf("viking/%s:%d", action, count),
		Result:     "success",
		Timestamp:  time.Now().Format(time.RFC3339),
		ActionID:   fmt.Sprintf("gc-%d", time.Now().Unix()),
	})
}

func (g *GC) cleanupEngrams() int {
	cutoff := time.Now().Add(-engramMaxAge)
	entries, err := os.ReadDir(g.engramDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			p := filepath.Join(g.engramDir, e.Name())
			if err := os.Remove(p); err == nil {
				n++
			}
		}
	}
	return n
}

// Start runs GC hourly.
func (g *GC) Start() {
	go func() {
		ticker := time.NewTicker(gcInterval)
		defer ticker.Stop()
		for range ticker.C {
			g.Run()
		}
	}()
}
