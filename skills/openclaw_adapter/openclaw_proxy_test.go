package openclaw_adapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"harmonclaw/skills"
)

func TestAdapter_Execute_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":{"answer":"ok"}}`))
	}))
	defer srv.Close()

	os.Setenv("HC_OPENCLAW_ENDPOINT", srv.URL)
	defer os.Unsetenv("HC_OPENCLAW_ENDPOINT")

	a := &Adapter{}
	out := a.Execute(skills.SkillInput{
		TraceID: "t1",
		Text:    "hello",
		Args:    map[string]string{},
	})
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s: %s", out.Status, out.Error)
	}
	var d map[string]any
	if json.Unmarshal(out.Data, &d) != nil {
		t.Fatal("invalid JSON")
	}
	if d["answer"] != "ok" {
		t.Errorf("want answer ok, got %v", d)
	}
}

func TestAdapter_Execute_Unreachable(t *testing.T) {
	os.Setenv("HC_OPENCLAW_ENDPOINT", "http://127.0.0.1:19999")
	defer os.Unsetenv("HC_OPENCLAW_ENDPOINT")

	a := &Adapter{}
	out := a.Execute(skills.SkillInput{TraceID: "t2", Text: "x"})
	if out.Status != "error" {
		t.Errorf("want error, got %s", out.Status)
	}
}

func TestHcToOpenClaw(t *testing.T) {
	hc := map[string]any{"text": "hi", "args": map[string]string{"a": "b"}}
	oc := hcToOpenClaw(hc)
	if oc["query"] != "hi" {
		t.Errorf("query: want hi, got %v", oc["query"])
	}
}
