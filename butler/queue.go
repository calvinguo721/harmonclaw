// Package butler (queue) provides RealtimeQueue for TTS/SSE priority.
package butler

import (
	"sync"
)

const queueCap = 16

type MsgType int

const (
	MsgNormal MsgType = iota
	MsgTTS
	MsgSSE
)

func (t MsgType) Priority() bool {
	return t == MsgTTS || t == MsgSSE
}

type QueueItem struct {
	Type    MsgType
	Payload any
}

type RealtimeQueue struct {
	prio  []QueueItem
	norm  []QueueItem
	mu    sync.Mutex
	avail chan struct{}
}

func NewRealtimeQueue() *RealtimeQueue {
	q := &RealtimeQueue{
		prio:  make([]QueueItem, 0, queueCap),
		norm:  make([]QueueItem, 0, queueCap),
		avail: make(chan struct{}, queueCap),
	}
	for i := 0; i < queueCap; i++ {
		q.avail <- struct{}{}
	}
	return q
}

func (q *RealtimeQueue) Enqueue(item QueueItem) bool {
	select {
	case <-q.avail:
	default:
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if item.Type.Priority() {
		q.prio = append(q.prio, item)
	} else {
		q.norm = append(q.norm, item)
	}
	return true
}

func (q *RealtimeQueue) Dequeue() (QueueItem, bool) {
	q.mu.Lock()
	var item QueueItem
	var ok bool
	if len(q.prio) > 0 {
		item = q.prio[0]
		q.prio = q.prio[1:]
		ok = true
	} else if len(q.norm) > 0 {
		item = q.norm[0]
		q.norm = q.norm[1:]
		ok = true
	}
	q.mu.Unlock()
	if ok {
		q.avail <- struct{}{}
	}
	return item, ok
}

func (q *RealtimeQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.prio) + len(q.norm)
}
