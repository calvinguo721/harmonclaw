// Package viking provides audit ledger for action tracing.
package viking

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
)

// LedgerEntry 等保 7 字段审计格式
type LedgerEntry struct {
	OperatorID string `json:"operator_id"`
	ActionType string `json:"action_type"`
	Resource   string `json:"resource"`
	Result     string `json:"result"` // success | fail
	ClientIP   string `json:"client_ip"`
	Timestamp  string `json:"timestamp"`
	ActionID   string `json:"action_id"`
}

type Ledger interface {
	Record(entry LedgerEntry)
	Latest(n int) ([]LedgerEntry, error)
	TraceByActionID(actionID string) ([]LedgerEntry, error)
	Close()
}

// LedgerQueryFilter for audit queries.
type LedgerQueryFilter struct {
	TimeFrom   string // RFC3339
	TimeTo     string
	OperatorID string
	ActionType string
	Resource   string
}

// QueryableLedger supports filtered queries.
type QueryableLedger interface {
	Ledger
	Query(filter LedgerQueryFilter) ([]LedgerEntry, error)
}

type FileLedger struct {
	fpath  string
	ch     chan LedgerEntry
	closed atomic.Bool
}

// NewFileLedger creates a ledger. If ledgerDir is empty, uses ~/.harmonclaw/ledger.
func NewFileLedger(ledgerDir string) (*FileLedger, error) {
	if ledgerDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		ledgerDir = filepath.Join(home, ".harmonclaw", "ledger")
	}
	if err := os.MkdirAll(ledgerDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir ledger: %w", err)
	}

	l := &FileLedger{
		fpath: filepath.Join(ledgerDir, "ledger.jsonl"),
		ch:    make(chan LedgerEntry, 64),
	}
	go l.drain()
	return l, nil
}

func (l *FileLedger) Record(entry LedgerEntry) {
	if l.closed.Load() {
		return
	}
	defer func() { _ = recover() }() // ignore panic from send on closed channel
	select {
	case l.ch <- entry:
	default:
		log.Printf("ledger channel full, dropping entry: %s", entry.ActionType)
	}
}

func (l *FileLedger) Close() {
	if l.closed.Swap(true) {
		return
	}
	close(l.ch)
}

func (l *FileLedger) drain() {
	for entry := range l.ch {
		data, err := json.Marshal(entry)
		if err != nil {
			log.Printf("ledger marshal: %v", err)
			continue
		}
		if err := LedgerSafeAppend(l.fpath, data); err != nil {
			log.Printf("ledger write: %v", err)
		}
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
		jsonPart := ledgerLineJSON(line)
		if len(jsonPart) == 0 {
			continue
		}
		var e LedgerEntry
		if err := json.Unmarshal(jsonPart, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func ledgerLineJSON(line []byte) []byte {
	s := string(line)
	if idx := strings.Index(s, "\t"); idx >= 0 {
		return []byte(s[:idx])
	}
	return line
}

func (l *FileLedger) TraceByActionID(actionID string) ([]LedgerEntry, error) {
	data, err := os.ReadFile(l.fpath)
	if err != nil {
		if os.IsNotExist(err) {
			return []LedgerEntry{}, nil
		}
		return nil, fmt.Errorf("read ledger: %w", err)
	}

	lines := bytes.Split(data, []byte("\n"))
	var entries []LedgerEntry
	for _, line := range lines {
		jsonPart := ledgerLineJSON(line)
		if len(jsonPart) == 0 {
			continue
		}
		var e LedgerEntry
		if err := json.Unmarshal(jsonPart, &e); err != nil {
			continue
		}
		if e.ActionID == actionID {
			entries = append(entries, e)
		}
	}
	sortByTimestamp(entries)
	return entries, nil
}

func sortByTimestamp(entries []LedgerEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp < entries[j].Timestamp
	})
}

// Query returns entries matching the filter.
func (l *FileLedger) Query(filter LedgerQueryFilter) ([]LedgerEntry, error) {
	data, err := os.ReadFile(l.fpath)
	if err != nil {
		if os.IsNotExist(err) {
			return []LedgerEntry{}, nil
		}
		return nil, fmt.Errorf("read ledger: %w", err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	var entries []LedgerEntry
	for _, line := range lines {
		jsonPart := ledgerLineJSON(line)
		if len(jsonPart) == 0 {
			continue
		}
		var e LedgerEntry
		if json.Unmarshal(jsonPart, &e) != nil {
			continue
		}
		if filter.OperatorID != "" && e.OperatorID != filter.OperatorID {
			continue
		}
		if filter.ActionType != "" && e.ActionType != filter.ActionType {
			continue
		}
		if filter.Resource != "" && e.Resource != filter.Resource {
			continue
		}
		if filter.TimeFrom != "" && e.Timestamp < filter.TimeFrom {
			continue
		}
		if filter.TimeTo != "" && e.Timestamp > filter.TimeTo {
			continue
		}
		entries = append(entries, e)
	}
	sortByTimestamp(entries)
	return entries, nil
}
