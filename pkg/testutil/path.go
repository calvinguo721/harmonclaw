// Package testutil provides helpers for tests.
package testutil

import (
	"os"
	"path/filepath"
	"runtime"
)

// ProjectRoot returns the path to the project root (directory containing go.mod).
// Uses runtime.Caller to locate the calling test file, then walks up to find go.mod.
func ProjectRoot() string {
	_, f, _, _ := runtime.Caller(0)
	dir := filepath.Dir(f)
	// Walk up from pkg/testutil to find go.mod
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

// ConfigPath returns the path to a config file under configs/.
func ConfigPath(name string) string {
	return filepath.Join(ProjectRoot(), "configs", name)
}
