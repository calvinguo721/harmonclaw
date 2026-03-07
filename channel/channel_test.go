package channel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPChannel_ServeHTTP(t *testing.T) {
	ch := NewHTTPChannel()
	body := `{"text":"hello","user_id":"u1"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.RemoteAddr = "1.2.3.4:5678"
	w := httptest.NewRecorder()

	ch.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status: %d", w.Code)
	}
	ctx := context.Background()
	in, _ := ch.Receive(ctx)
	msg := <-in
	if msg.Text != "hello" || msg.UserID != "u1" {
		t.Errorf("msg: %+v", msg)
	}
}

func TestWSChannel_ID(t *testing.T) {
	w := NewWSChannel()
	if w.ID() != "websocket" {
		t.Errorf("id: %s", w.ID())
	}
}

func TestWeChatChannel_ID(t *testing.T) {
	w := NewWeChatChannel()
	if w.ID() != "wechat" {
		t.Errorf("id: %s", w.ID())
	}
}
