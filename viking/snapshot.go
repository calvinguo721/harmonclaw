// Package viking (snapshot) provides hourly tar.gz snapshots, keep 24, Snapshot/Restore/ListSnapshots.
package viking

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SnapshotManager keeps last 24 hourly tar.gz snapshots.
type SnapshotManager struct {
	mu       sync.Mutex
	dir      string
	srcDir   string
	maxKeep  int
	versions []string
}

// NewSnapshotManager creates a manager. srcDir is the directory to snapshot.
func NewSnapshotManager(snapDir, srcDir string, maxKeep int) *SnapshotManager {
	if maxKeep < 1 {
		maxKeep = 24
	}
	return &SnapshotManager{
		dir:     snapDir,
		srcDir:  srcDir,
		maxKeep: maxKeep,
	}
}

// Snapshot creates a tar.gz of srcDir. Returns path.
func (s *SnapshotManager) Snapshot() (string, error) {
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102150405")
	name := fmt.Sprintf("snap_%s.tar.gz", ts)
	path := filepath.Join(s.dir, name)
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	err = filepath.Walk(s.srcDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(s.srcDir, p)
		if rel == "." {
			return nil
		}
		rel = filepath.Join(string(filepath.Separator), rel)
		h, _ := tar.FileInfoHeader(fi, "")
		h.Name = rel
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if fi.Mode().IsRegular() {
			r, _ := os.Open(p)
			io.Copy(tw, r)
			r.Close()
		}
		return nil
	})
	if err != nil {
		tw.Close()
		gw.Close()
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	tw.Close()
	gw.Close()
	f.Close()
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

// Restore extracts tar.gz at path to destDir.
func (s *SnapshotManager) Restore(path, destDir string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(h.Name, string(filepath.Separator))
		target := filepath.Join(destDir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			w, _ := os.Create(target)
			io.Copy(w, tr)
			w.Close()
		}
	}
	return nil
}

// ListSnapshots returns snapshot paths, newest first.
func (s *SnapshotManager) ListSnapshots() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshVersions()
	out := make([]string, len(s.versions))
	for i, v := range s.versions {
		out[i] = filepath.Join(s.dir, v)
	}
	return out
}

func (s *SnapshotManager) refreshVersions() {
	entries, _ := os.ReadDir(s.dir)
	s.versions = nil
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".tar.gz") {
			s.versions = append(s.versions, e.Name())
		}
	}
	sort.Slice(s.versions, func(i, j int) bool { return s.versions[i] > s.versions[j] })
	if len(s.versions) > s.maxKeep {
		s.versions = s.versions[:s.maxKeep]
	}
}
