// Package viking provides path helpers for engrams.
package viking

import (
	"os"
	"path/filepath"
)

// EngramPath returns path for engram file. If baseDir is empty, uses ~/.harmonclaw/viking/engrams.
func EngramPath(filename string) (string, error) {
	return EngramPathWithBase("", filename)
}

func EngramPathWithBase(baseDir, filename string) (string, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		baseDir = filepath.Join(home, ".harmonclaw", "viking")
	}
	dir := filepath.Join(baseDir, "engrams")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}
