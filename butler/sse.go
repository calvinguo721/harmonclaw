// Package butler (sse) provides SSE engine with heartbeat, reconnect, backpressure.
package butler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// WriteEvent writes a single SSE event.
func (s *SSEWriter) WriteEvent(event, data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return io.ErrClosedPipe
	}
	if event != "" {
		fmt.Fprintf(s.w, "event: %s\n", event)
	}
	fmt.Fprintf(s.w, "data: %s\n\n", data)
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

// WriteJSON writes data as JSON event.
func (s *SSEWriter) WriteJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.WriteEvent("", string(b))
}

// Close marks writer closed.
func (s *SSEWriter) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}

// SSEStream manages a stream with heartbeat and backpressure.
type SSEStream struct {
	writer   *SSEWriter
	heartbeat time.Duration
	done      chan struct{}
	mu        sync.Mutex
	pending   int
	limit     int
}

// NewSSEStream creates a stream.
func NewSSEStream(w http.ResponseWriter, heartbeat time.Duration) *SSEStream {
	if heartbeat <= 0 {
		heartbeat = defaultHeartbeatInterval
	}
	s := &SSEStream{
		writer:   NewSSEWriter(w),
		heartbeat: heartbeat,
		done:     make(chan struct{}),
		limit:    defaultBackpressureLimit,
	}
	go s.heartbeatLoop()
	return s
}

func (s *SSEStream) heartbeatLoop() {
	ticker := time.NewTicker(s.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.writer.WriteEvent("heartbeat", `{"ts":"`+time.Now().Format(time.RFC3339)+`"}`)
		}
	}
}

// Send sends data. Returns error if backpressure limit exceeded.
func (s *SSEStream) Send(data string) error {
	s.mu.Lock()
	if s.pending >= s.limit {
		s.mu.Unlock()
		return fmt.Errorf("backpressure: queue full")
	}
	s.pending++
	s.mu.Unlock()
	err := s.writer.WriteEvent("", data)
	s.mu.Lock()
	s.pending--
	s.mu.Unlock()
	return err
}

// Stop stops heartbeat.
func (s *SSEStream) Stop() {
	close(s.done)
	s.writer.Close()
}
