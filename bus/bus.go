// Package bus provides inter-core message passing and topic-based event pub/sub.
package bus

import (
	"sync"
)

const (
	bufSize   = 64
	eventBuf  = 100
)

var ch = make(chan Message, bufSize)

// defaultDepthController limits spawn depth and children per core.
var defaultDepthController = NewDepthController()

// GetDepthController returns the default depth controller for spawn limits.
func GetDepthController() *DepthController {
	return defaultDepthController
}

type CoreID string

const (
	Governor  CoreID = "governor"
	Butler    CoreID = "butler"
	Architect CoreID = "architect"
)

// Event types for pub/sub
const (
	EventSovereigntyChanged = "sovereignty.changed"
	EventConfigReloaded     = "config.reloaded"
	EventSkillDegraded      = "skill.degraded"
	EventSkillRecovered     = "skill.recovered"
	EventAuthFailed         = "auth.failed"
)

type Message struct {
	From    CoreID
	To      CoreID
	Type    string
	Payload any
	TraceID string
}

func Send(m Message) {
	select {
	case ch <- m:
	default:
	}
}

func Subscribe() <-chan Message {
	return ch
}

// --- topic-based event bus ---

type eventMsg struct {
	topic   string
	payload any
}

var (
	eventCh     = make(chan eventMsg, eventBuf)
	subscribers = make(map[string][]chan any)
	subMu       sync.RWMutex
)

func Publish(topic string, payload any) {
	select {
	case eventCh <- eventMsg{topic: topic, payload: payload}:
	default:
	}
}

func SubscribeTopic(topic string, handler func(payload any)) func() {
	ch := make(chan any, 8)
	subMu.Lock()
	subscribers[topic] = append(subscribers[topic], ch)
	subMu.Unlock()

	go func() {
		for p := range ch {
			handler(p)
		}
	}()

	return func() {
		subMu.Lock()
		defer subMu.Unlock()
		for i, c := range subscribers[topic] {
			if c == ch {
				close(ch)
				subscribers[topic] = append(subscribers[topic][:i], subscribers[topic][i+1:]...)
				return
			}
		}
	}
}

func init() {
	go func() {
		for ev := range eventCh {
			subMu.RLock()
			subs := append([]chan any(nil), subscribers[ev.topic]...)
			subMu.RUnlock()
			for _, c := range subs {
				select {
				case c <- ev.payload:
				default:
				}
			}
		}
	}()
}
