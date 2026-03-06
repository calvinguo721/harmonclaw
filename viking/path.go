// Package viking provides path helpers for engrams.
package viking

import (
	"os"
	"path/filepath"
)

func EngramPath(filename string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".harmonclaw", "viking", "engrams")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}
