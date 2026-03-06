// Package viking provides memory store and file persistence.
package viking

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Memory interface {
	SaveMemory(user, sessionID, role, content string) error
}

type FileStore struct {
	baseDir string
}

// NewFileStore creates memory store. If baseDir is empty, uses ~/.harmonclaw/viking.
func NewFileStore(baseDir string) (*FileStore, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		baseDir = filepath.Join(home, ".harmonclaw", "viking")
	}
	return &FileStore{baseDir: baseDir}, nil
}

func (fs *FileStore) SaveMemory(user, sessionID, role, content string) error {
	date := time.Now().Format("2006-01-02")
	dir := filepath.Join(fs.baseDir, user, date)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	fpath := filepath.Join(dir, sessionID+".txt")
	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open %s: %w", fpath, err)
	}
	defer f.Close()

	ts := time.Now().Format("15:04:05")
	line := fmt.Sprintf("[%s] %s: %s\n", ts, role, content)
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("write %s: %w", fpath, err)
	}
	return nil
}
