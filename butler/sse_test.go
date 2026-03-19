package butler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSSEWriter_WriteEvent(t *testing.T) {
	rr := httptest.NewRecorder()
	sw := NewSSEWriter(rr)
	id, err := sw.WriteEvent("test", `{"x":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Errorf("event id: want 1, got %d", id)
	}
	if rr.Body.Len() == 0 {
		t.Error("body should have content")
	}
	sw.Close()
	_, err = sw.WriteEvent("x", "y")
	if err == nil {
		t.Error("write after close should fail")
	}
}

func TestSSEStream_Heartbeat(t *testing.T) {
	rr := httptest.NewRecorder()
	stream := NewSSEStream(rr, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	stream.StartWithContext(ctx)
	time.Sleep(120 * time.Millisecond)
	cancel()
	stream.Stop()
	if rr.Body.Len() == 0 {
		t.Error("heartbeat should have written")
	}
}

func TestSSEStream_Backpressure(t *testing.T) {
	rr := httptest.NewRecorder()
	stream := NewSSEStream(rr, time.Hour)
	for i := 0; i < 10; i++ {
		stream.Send(`{"n":` + string(rune('0'+i%10)) + `}`)
	}
	stream.Stop()
	if rr.Body.Len() == 0 {
		t.Error("Send should write events")
	}
}

func TestLastEventID(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Last-Event-ID", "42")
	if LastEventID(req) != "42" {
		t.Error("LastEventID should return header value")
	}
}

func TestParseLastEventID(t *testing.T) {
	if ParseLastEventID("99") != 99 {
		t.Error("ParseLastEventID 99")
	}
	if ParseLastEventID("") != 0 {
		t.Error("ParseLastEventID empty")
	}
}

