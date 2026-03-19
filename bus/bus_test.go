package bus

import (
	"sync"
	"testing"
	"time"
)

func TestSendSubscribe(t *testing.T) {
	ch := Subscribe()
	m := Message{From: Governor, Type: "pulse", Payload: "ok"}
	Send(m)
	got := <-ch
	if got.From != Governor || got.Type != "pulse" {
		t.Errorf("got %+v", got)
	}
}

func TestPublishSubscribe(t *testing.T) {
	var received []any
	var mu sync.Mutex
	unsub := SubscribeTopic(EventConfigReloaded, func(p any) {
		mu.Lock()
		received = append(received, p)
		mu.Unlock()
	})
	defer unsub()

	Publish(EventConfigReloaded, map[string]string{"path": "configs"})
	Publish(EventConfigReloaded, "reload")

	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	n := len(received)
	mu.Unlock()
	if n < 1 {
		t.Errorf("want >=1 received, got %d", n)
	}
}
