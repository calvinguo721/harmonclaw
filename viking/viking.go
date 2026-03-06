// Package viking provides memory store and file persistence.
package viking

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Memory interface {
	SaveMemory(user, sessionID, role, content string) error
}

// HistoryEntry for conversation history.
type HistoryEntry struct {
	Role    string
	Content string
}

// MemoryWithHistory extends Memory with history load.
type MemoryWithHistory interface {
	Memory
	LoadHistory(user, sessionID string) ([]HistoryEntry, error)
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

var historyLineRe = regexp.MustCompile(`^\[\d{2}:\d{2}:\d{2}\] (\w+): (.*)`)

// LoadHistory reads conversation history for user/session. Scans last 7 days.
func (fs *FileStore) LoadHistory(user, sessionID string) ([]HistoryEntry, error) {
	var entries []HistoryEntry
	base := filepath.Join(fs.baseDir, user)
	for d := 0; d < 7; d++ {
		date := time.Now().AddDate(0, 0, -d).Format("2006-01-02")
		fpath := filepath.Join(base, date, sessionID+".txt")
		f, err := os.Open(fpath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			m := historyLineRe.FindStringSubmatch(sc.Text())
			if len(m) == 3 {
				entries = append(entries, HistoryEntry{Role: m[1], Content: strings.TrimSpace(m[2])})
			}
		}
		f.Close()
		if len(entries) > 0 {
			break
		}
	}
	return entries, nil
}
