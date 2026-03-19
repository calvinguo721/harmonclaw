package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestEdgeHealth(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer backend.Close()
	os.Setenv("HC_BACKEND_URL", backend.URL)
	defer os.Unsetenv("HC_BACKEND_URL")

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handleHealth(w, req)
	if w.Code != 200 {
		t.Errorf("health: want 200, got %d", w.Code)
	}
}

func TestEdgeCache(t *testing.T) {
	cacheAdd("s1", "user", "hello")
	cacheAdd("s1", "assistant", "hi")
	entries := cacheGet()
	if len(entries) != 2 {
		t.Errorf("cache: want 2, got %d", len(entries))
	}
}
