// Package butler (tts_stream) provides TTS streaming with audio chunk push.
package butler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"sync"
)

// TTSChunk represents an audio chunk for streaming.
type TTSChunk struct {
	Index int    `json:"index"`
	Data  string `json:"data"` // base64 PCM
	Done  bool   `json:"done,omitempty"`
}

// TTSStreamer manages TTS chunk streaming.
type TTSStreamer struct {
	mu     sync.Mutex
	chunks []TTSChunk
	index  int
}

// NewTTSStreamer creates a streamer.
func NewTTSStreamer() *TTSStreamer {
	return &TTSStreamer{chunks: make([]TTSChunk, 0, 32)}
}

// Push adds an audio chunk (raw bytes, will be base64 encoded).
func (t *TTSStreamer) Push(data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.index++
	t.chunks = append(t.chunks, TTSChunk{
		Index: t.index,
		Data:  base64.StdEncoding.EncodeToString(data),
	})
}

// PushBase64 adds pre-encoded chunk.
func (t *TTSStreamer) PushBase64(b64 string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.index++
	t.chunks = append(t.chunks, TTSChunk{Index: t.index, Data: b64})
}

// Finalize marks stream done.
func (t *TTSStreamer) Finalize() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.chunks) > 0 {
		t.chunks[len(t.chunks)-1].Done = true
	}
}

// Chunks returns all chunks.
func (t *TTSStreamer) Chunks() []TTSChunk {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TTSChunk, len(t.chunks))
	copy(out, t.chunks)
	return out
}

// ToJSON returns chunks as JSON array.
func (t *TTSStreamer) ToJSON() ([]byte, error) {
	return json.Marshal(t.Chunks())
}

// DecodeChunk decodes base64 data from chunk.
func DecodeChunk(c TTSChunk) ([]byte, error) {
	return base64.StdEncoding.DecodeString(c.Data)
}

// TTSBuffer accumulates chunks for playback.
type TTSBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	chunks int
}

// NewTTSBuffer creates a buffer.
func NewTTSBuffer() *TTSBuffer {
	return &TTSBuffer{}
}

// Append adds decoded audio.
func (b *TTSBuffer) Append(pcm []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Write(pcm)
	b.chunks++
}

// Len returns byte length.
func (b *TTSBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

// Bytes returns copy of buffer.
func (b *TTSBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Bytes()
}
