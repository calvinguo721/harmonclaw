package edge

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOfflineManager_Check(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	mgr := NewOfflineManager(srv.URL)
	if mgr.Check() {
		t.Error("expected online when server is up")
	}
	if mgr.IsOffline() {
		t.Error("expected not offline")
	}
}

func TestOfflineManager_Offline(t *testing.T) {
	mgr := NewOfflineManager("http://127.0.0.1:19999")
	mgr.Check()
	if !mgr.IsOffline() {
		t.Error("expected offline when server unreachable")
	}
}

func TestOfflineManager_RecordGetCache(t *testing.T) {
	mgr := NewOfflineManager("http://x")
	mgr.Record("s1", "user", "hello")
	mgr.Record("s1", "assistant", "hi")
	cache := mgr.GetCache()
	if len(cache) != 2 {
		t.Errorf("want 2 cached, got %d", len(cache))
	}
}
