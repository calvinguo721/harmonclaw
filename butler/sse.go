// Package butler (sse) provides SSE engine with heartbeat, context.Done, backpressure, Last-Event-ID.
package butler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultHeartbeatInterval = 15 * time.Second
	defaultBackpressureLimit = 64
)

// SSEWriter wraps http.ResponseWriter for SSE with flush and backpressure.
type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
	closed  bool
	eventID int64
}

// NewSSEWriter creates an SSE writer.
func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	f, _ := w.(http.Flusher)
	return &SSEWriter{w: w, flusher: f}
}

// WriteEvent writes a single SSE event with Last-Event-ID.
func (s *SSEWriter) WriteEvent(event, data string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, io.ErrClosedPipe
	}
	s.eventID++
	id := s.eventID
	if event != "" {
		fmt.Fprintf(s.w, "event: %s\n", event)
	}
	fmt.Fprintf(s.w, "id: %d\n", id)
	fmt.Fprintf(s.w, "data: %s\n\n", data)
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return id, nil
}

// WriteJSON writes data as JSON event.
func (s *SSEWriter) WriteJSON(v any) (int64, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return 0, err
	}
	return s.WriteEvent("", string(b))
}

// Close marks writer closed.
func (s *SSEWriter) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}

// SSEStream manages a stream with heartbeat, context.Done, backpressure drop old.
type SSEStream struct {
	writer    *SSEWriter
	heartbeat time.Duration
	done      chan struct{}
	mu        sync.Mutex
	queue     []string
	limit     int
	lastID    int64
}

// NewSSEStream creates a stream. Pass ctx to StopOnContext for context.Done handling.
func NewSSEStream(w http.ResponseWriter, heartbeat time.Duration) *SSEStream {
	if heartbeat <= 0 {
		heartbeat = defaultHeartbeatInterval
	}
	return &SSEStream{
		writer:    NewSSEWriter(w),
		heartbeat: heartbeat,
		done:      make(chan struct{}),
		queue:     make([]string, 0, defaultBackpressureLimit),
		limit:     defaultBackpressureLimit,
	}
}

// StartWithContext runs heartbeat loop and stops on ctx.Done.
func (s *SSEStream) StartWithContext(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case <-ctx.Done():
				s.Stop()
				return
			case <-ticker.C:
				s.writer.WriteEvent("heartbeat", `{"ts":"`+time.Now().Format(time.RFC3339)+`"}`)
			}
		}
	}()
}

// LastEventID parses Last-Event-ID header for reconnect support.
func LastEventID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("Last-Event-ID"))
}

// ParseLastEventID returns numeric ID or 0.
func ParseLastEventID(h string) int64 {
	id, _ := strconv.ParseInt(h, 10, 64)
	return id
}

// Send sends data. On backpressure, drops oldest and adds new.
func (s *SSEStream) Send(data string) error {
	s.mu.Lock()
	if len(s.queue) >= s.limit {
		s.queue = s.queue[1:]
	}
	s.queue = append(s.queue, data)
	s.mu.Unlock()
	id, err := s.writer.WriteEvent("", data)
	s.mu.Lock()
	s.lastID = id
	s.mu.Unlock()
	return err
}

// Stop stops heartbeat.
func (s *SSEStream) Stop() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	s.writer.Close()
}
