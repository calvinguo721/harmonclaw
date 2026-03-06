// Package viking (snapshot) provides timed snapshots with version retention.
package viking

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// SnapshotManager keeps last N versions of data.
type SnapshotManager struct {
	mu       sync.Mutex
	dir      string
	prefix   string
	maxKeep  int
	versions []string
}

// NewSnapshotManager creates a manager.
func NewSnapshotManager(dir, prefix string, maxKeep int) *SnapshotManager {
	if maxKeep < 1 {
		maxKeep = 5
	}
	return &SnapshotManager{
		dir:     dir,
		prefix:  prefix,
		maxKeep: maxKeep,
	}
}

// Save writes a snapshot. Returns path.
func (s *SnapshotManager) Save(data []byte) (string, error) {
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102150405")
	name := fmt.Sprintf("%s_%s.json", s.prefix, ts)
	path := filepath.Join(s.dir, name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	s.mu.Lock()
	s.versions = append(s.versions, name)
	sort.Slice(s.versions, func(i, j int) bool { return s.versions[i] > s.versions[j] })
	if len(s.versions) > s.maxKeep {
		for _, v := range s.versions[s.maxKeep:] {
			os.Remove(filepath.Join(s.dir, v))
		}
		s.versions = s.versions[:s.maxKeep]
	}
	s.mu.Unlock()
	return path, nil
}

// SaveJSON marshals v and saves.
func (s *SnapshotManager) SaveJSON(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return s.Save(data)
}

// Latest returns path to most recent snapshot.
func (s *SnapshotManager) Latest() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.versions) == 0 {
		return "", fmt.Errorf("no snapshots")
	}
	return filepath.Join(s.dir, s.versions[0]), nil
}

// List returns all snapshot paths, newest first.
func (s *SnapshotManager) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.versions))
	for i, v := range s.versions {
		out[i] = filepath.Join(s.dir, v)
	}
	return out
}
