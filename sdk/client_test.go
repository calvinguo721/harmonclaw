package sdk

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_Health(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"overall":"healthy","governor":{"status":"ok"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	h, err := c.Health()
	if err != nil {
		t.Fatal(err)
	}
	if h["overall"] != "healthy" {
		t.Errorf("want healthy, got %v", h["overall"])
	}
}
