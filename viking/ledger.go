package viking

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type LedgerEntry struct {
	Timestamp string `json:"timestamp"`
	Role      string `json:"role"`
	Action    string `json:"action"`
	Tokens    int    `json:"tokens"`
	Status    string `json:"status"`
}

type Ledger interface {
	Record(entry LedgerEntry)
	Latest(n int) ([]LedgerEntry, error)
	Close()
}

type FileLedger struct {
	fpath string
	ch    chan LedgerEntry
}

func NewFileLedger() (*FileLedger, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".harmonclaw", "viking")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir ledger: %w", err)
	}

	l := &FileLedger{
		fpath: filepath.Join(dir, "ledger.json"),
		ch:    make(chan LedgerEntry, 64),
	}
	go l.drain()
	return l, nil
}

func (l *FileLedger) Record(entry LedgerEntry) {
	select {
	case l.ch <- entry:
	default:
		log.Printf("ledger channel full, dropping entry: %s", entry.Action)
	}
}

func (l *FileLedger) Close() {
	close(l.ch)
}

func (l *FileLedger) drain() {
	for entry := range l.ch {
		data, err := json.Marshal(entry)
		if err != nil {
			log.Printf("ledger marshal: %v", err)
			continue
		}
		f, err := os.OpenFile(l.fpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("ledger open: %v", err)
			continue
		}
		f.Write(data)
		f.Write([]byte("\n"))
		f.Close()
	}
}

func (l *FileLedger) Latest(n int) ([]LedgerEntry, error) {
	data, err := os.ReadFile(l.fpath)
	if err != nil {
		if os.IsNotExist(err) {
			return []LedgerEntry{}, nil
		}
		return nil, fmt.Errorf("read ledger: %w", err)
	}

	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return []LedgerEntry{}, nil
	}

	lines := bytes.Split(data, []byte("\n"))
	start := 0
	if len(lines) > n {
		start = len(lines) - n
	}

	entries := make([]LedgerEntry, 0, n)
	for _, line := range lines[start:] {
		if len(line) == 0 {
			continue
		}
		var e LedgerEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}
