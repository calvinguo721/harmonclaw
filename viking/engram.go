// Package viking provides Engram structured memory format.
package viking

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const engramSeparator = "---"

// Engram is a structured memory unit.
type Engram struct {
	ID             string
	SessionID      string
	Timestamp      time.Time
	Content        string
	Summary        string
	Keywords       []string
	Entities       []string
	Importance     int
	Classification string
	ActionID       string
	LastAccessed   time.Time
	AccessCount    int
	Metadata       map[string]string
}

// EngramStore persists engrams to files.
type EngramStore struct {
	mu      sync.RWMutex
	baseDir string
	engrams map[string]Engram
	maxSize int
}

// NewEngramStore creates a store. baseDir defaults to data/viking/engrams.
func NewEngramStore(baseDir string) *EngramStore {
	if baseDir == "" {
		baseDir = "data/viking/engrams"
	}
	return &EngramStore{
		baseDir: baseDir,
		engrams: make(map[string]Engram),
		maxSize: 10000,
	}
}

// Write saves an engram to file and index.
func (s *EngramStore) Write(e Engram) error {
	if e.ID == "" {
		e.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	if e.LastAccessed.IsZero() {
		e.LastAccessed = e.Timestamp
	}
	date := e.Timestamp.Format("2006-01-02")
	dir := filepath.Join(s.baseDir, date, e.SessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	fpath := filepath.Join(dir, e.ID+".txt")
	data := serializeEngram(e)
	tmp := fpath + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, fpath); err != nil {
		os.Remove(tmp)
		return err
	}
	s.mu.Lock()
	s.engrams[e.ID] = e
	if len(s.engrams) > s.maxSize {
		s.gc()
	}
	s.mu.Unlock()
	return nil
}

func serializeEngram(e Engram) string {
	var b strings.Builder
	writeField(&b, "ID", e.ID)
	writeField(&b, "SessionID", e.SessionID)
	writeField(&b, "Timestamp", e.Timestamp.Format(time.RFC3339))
	writeField(&b, "Content", e.Content)
	writeField(&b, "Summary", e.Summary)
	writeField(&b, "Keywords", strings.Join(e.Keywords, ","))
	writeField(&b, "Entities", strings.Join(e.Entities, ","))
	writeField(&b, "Importance", fmt.Sprintf("%d", e.Importance))
	writeField(&b, "Classification", e.Classification)
	writeField(&b, "ActionID", e.ActionID)
	writeField(&b, "LastAccessed", e.LastAccessed.Format(time.RFC3339))
	writeField(&b, "AccessCount", fmt.Sprintf("%d", e.AccessCount))
	for k, v := range e.Metadata {
		writeField(&b, "meta:"+k, v)
	}
	b.WriteString(engramSeparator + "\n")
	return b.String()
}

func writeField(b *strings.Builder, key, val string) {
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(val)
	b.WriteString("\n")
}

// Load scans baseDir and loads all engrams into index.
func (s *EngramStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.engrams = make(map[string]Engram)
	return filepath.Walk(s.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".txt") || strings.HasSuffix(path, ".tmp") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		e, err := parseEngram(string(data))
		if err != nil {
			return nil
		}
		if e.ID == "" {
			e.ID = strings.TrimSuffix(filepath.Base(path), ".txt")
		}
		s.engrams[e.ID] = e
		return nil
	})
}

func parseEngram(data string) (Engram, error) {
	var e Engram
	e.Metadata = make(map[string]string)
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == engramSeparator {
			break
		}
		if idx := strings.Index(line, ": "); idx > 0 {
			key := line[:idx]
			val := strings.TrimSpace(line[idx+2:])
			switch key {
			case "ID":
				e.ID = val
			case "SessionID":
				e.SessionID = val
			case "Timestamp":
				e.Timestamp, _ = time.Parse(time.RFC3339, val)
			case "Content":
				e.Content = val
			case "Summary":
				e.Summary = val
			case "Keywords":
				if val != "" {
					e.Keywords = strings.Split(val, ",")
				}
			case "Entities":
				if val != "" {
					e.Entities = strings.Split(val, ",")
				}
			case "Importance":
				fmt.Sscanf(val, "%d", &e.Importance)
			case "Classification":
				e.Classification = val
			case "ActionID":
				e.ActionID = val
			case "LastAccessed":
				e.LastAccessed, _ = time.Parse(time.RFC3339, val)
			case "AccessCount":
				fmt.Sscanf(val, "%d", &e.AccessCount)
			default:
				if strings.HasPrefix(key, "meta:") {
					e.Metadata[strings.TrimPrefix(key, "meta:")] = val
				}
			}
		}
	}
	return e, nil
}

// Get returns engram by ID.
func (s *EngramStore) Get(id string) (Engram, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.engrams[id]
	return e, ok
}

// List returns all engrams (for GC).
func (s *EngramStore) List() []Engram {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Engram, 0, len(s.engrams))
	for _, e := range s.engrams {
		out = append(out, e)
	}
	return out
}

// Delete removes an engram.
func (s *EngramStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.engrams[id]
	if !ok {
		return nil
	}
	date := e.Timestamp.Format("2006-01-02")
	fpath := filepath.Join(s.baseDir, date, e.SessionID, id+".txt")
	os.Remove(fpath)
	delete(s.engrams, id)
	return nil
}

func (s *EngramStore) gc() {
	type scored struct {
		id    string
		score float64
	}
	var list []scored
	now := time.Now()
	for id, e := range s.engrams {
		recency := now.Sub(e.LastAccessed).Hours() / 24
		imp := float64(e.Importance)
		if imp < 1 {
			imp = 1
		}
		score := imp * (1 + recency/30)
		list = append(list, scored{id, score})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].score < list[j].score })
	evict := len(list) / 10
	if evict < 1 {
		evict = 1
	}
	for i := 0; i < evict && i < len(list); i++ {
		id := list[i].id
		e, ok := s.engrams[id]
		if ok {
			date := e.Timestamp.Format("2006-01-02")
			fpath := filepath.Join(s.baseDir, date, e.SessionID, id+".txt")
			os.Remove(fpath)
			delete(s.engrams, id)
		}
	}
}
