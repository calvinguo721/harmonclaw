// Package viking provides BM25 hybrid search.
package viking

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// BM25Doc holds document with metadata for scoring.
type BM25Doc struct {
	ID           string
	Content      string
	Timestamp    time.Time
	Importance   int
	LastAccessed time.Time
}

// BM25Index is a search index with BM25 scoring.
type BM25Index struct {
	mu         sync.RWMutex
	termDocFreq map[string]map[string]int // term -> docID -> freq
	docLen     map[string]int
	docMeta    map[string]BM25Doc
	avgDocLen  float64
	totalDocs  int
	segmenter  *Segmenter
	path       string
}

// NewBM25Index creates a BM25 index.
func NewBM25Index(dictPath string) *BM25Index {
	return &BM25Index{
		termDocFreq: make(map[string]map[string]int),
		docLen:      make(map[string]int),
		docMeta:     make(map[string]BM25Doc),
		segmenter:   NewSegmenter(dictPath),
	}
}

// NewBM25IndexWithPath creates and loads from path.
func NewBM25IndexWithPath(indexPath, dictPath string) *BM25Index {
	idx := NewBM25Index(dictPath)
	idx.path = indexPath
	idx.Load()
	return idx
}

type bm25Line struct {
	DocID     string            `json:"doc_id"`
	Content   string            `json:"content"`
	Timestamp string            `json:"timestamp"`
	Importance int              `json:"importance"`
	Meta      map[string]string `json:"meta"`
}

// Load reads index from JSON.
func (b *BM25Index) Load() error {
	if b.path == "" {
		return nil
	}
	data, err := os.ReadFile(b.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		var line bm25Line
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		tokens := b.segmenter.Segment(strings.ToLower(line.Content))
		b.docLen[line.DocID] = len(tokens)
		ts, _ := time.Parse(time.RFC3339, line.Timestamp)
		b.docMeta[line.DocID] = BM25Doc{
			ID:         line.DocID,
			Content:    line.Content,
			Timestamp:  ts,
			Importance: line.Importance,
		}
		for _, t := range tokens {
			if t == "" {
				continue
			}
			if b.termDocFreq[t] == nil {
				b.termDocFreq[t] = make(map[string]int)
			}
			b.termDocFreq[t][line.DocID]++
		}
	}
	b.recomputeAvgLen()
	return nil
}

// Persist writes index to JSON.
func (b *BM25Index) Persist() error {
	if b.path == "" {
		return nil
	}
	b.mu.RLock()
	lines := make([]bm25Line, 0, len(b.docMeta))
	for id, d := range b.docMeta {
		lines = append(lines, bm25Line{
			DocID:      id,
			Content:    d.Content,
			Timestamp:  d.Timestamp.Format(time.RFC3339),
			Importance: d.Importance,
		})
	}
	b.mu.RUnlock()
	dir := filepath.Dir(b.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := b.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, l := range lines {
		if err := enc.Encode(l); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, b.path)
}

func (b *BM25Index) recomputeAvgLen() {
	var total int
	for _, l := range b.docLen {
		total += l
	}
	b.totalDocs = len(b.docLen)
	if b.totalDocs > 0 {
		b.avgDocLen = float64(total) / float64(b.totalDocs)
	} else {
		b.avgDocLen = 1
	}
}

// Index adds or updates a document.
func (b *BM25Index) Index(docID, content string, meta map[string]string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if old, ok := b.docMeta[docID]; ok {
		for _, t := range b.segmenter.Segment(strings.ToLower(old.Content)) {
			if m := b.termDocFreq[t]; m != nil {
				delete(m, docID)
				if len(m) == 0 {
					delete(b.termDocFreq, t)
				}
			}
		}
		delete(b.docLen, docID)
		delete(b.docMeta, docID)
	}
	imp := 3
	if v, ok := meta["importance"]; ok && len(v) > 0 {
		switch v[0] {
		case '1', '2', '3', '4', '5':
			imp = int(v[0] - '0')
		}
	}
	tokens := b.segmenter.Segment(strings.ToLower(content))
	b.docLen[docID] = len(tokens)
	b.docMeta[docID] = BM25Doc{
		ID:        docID,
		Content:   content,
		Timestamp: time.Now(),
		Importance: imp,
	}
	for _, t := range tokens {
		if t == "" {
			continue
		}
		if b.termDocFreq[t] == nil {
			b.termDocFreq[t] = make(map[string]int)
		}
		b.termDocFreq[t][docID]++
	}
	b.recomputeAvgLen()
}

// Search returns docIDs sorted by BM25 score * time decay * importance.
func (b *BM25Index) Search(query string, limit int) []string {
	tokens := b.segmenter.Segment(strings.ToLower(query))
	if len(tokens) == 0 {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	scores := make(map[string]float64)
	now := time.Now()
	for _, t := range tokens {
		postings, ok := b.termDocFreq[t]
		if !ok {
			continue
		}
		nq := len(postings)
		idf := math.Log((float64(b.totalDocs)-float64(nq)+0.5)/(float64(nq)+0.5) + 1)
		for docID, freq := range postings {
			docLen := float64(b.docLen[docID])
			if docLen < 1 {
				docLen = 1
			}
			bm25 := idf * (float64(freq) * (bm25K1 + 1)) / (float64(freq) + bm25K1*(1-bm25B+bm25B*docLen/b.avgDocLen))
			meta := b.docMeta[docID]
			decay := 1.0
			if meta.LastAccessed.Before(now.AddDate(0, 0, -30)) {
				decay = 0.5
			}
			imp := float64(meta.Importance) / 5.0
			if imp < 0.2 {
				imp = 0.2
			}
			scores[docID] += bm25 * decay * imp
		}
	}
	type scored struct {
		id    string
		score float64
	}
	var list []scored
	for id, sc := range scores {
		if sc > 0 {
			list = append(list, scored{id, sc})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })
	out := make([]string, 0, limit)
	for i := 0; i < limit && i < len(list); i++ {
		out = append(out, list[i].id)
	}
	return out
}

// Get returns document by ID.
func (b *BM25Index) Get(docID string) (BM25Doc, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	d, ok := b.docMeta[docID]
	return d, ok
}

// UpdateAccess updates last accessed time.
func (b *BM25Index) UpdateAccess(docID string, t time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if d, ok := b.docMeta[docID]; ok {
		d.LastAccessed = t
		b.docMeta[docID] = d
	}
}
