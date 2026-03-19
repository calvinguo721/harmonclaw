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
		if r.URL.Path != "/tools/invoke" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
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

func TestAdapter_RequestFormat(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":{"answer":"ok"}}`))
	}))
	defer srv.Close()
	os.Setenv("HC_OPENCLAW_ENDPOINT", srv.URL)
	defer os.Unsetenv("HC_OPENCLAW_ENDPOINT")

	a := &Adapter{}
	a.Execute(skills.SkillInput{TraceID: "t1", Text: "hi", Args: map[string]string{"a": "b"}})
	if gotBody["tool"] != "openclaw" || gotBody["action"] != "invoke" {
		t.Errorf("want tool=openclaw action=invoke, got %v", gotBody)
	}
	if gotBody["sessionKey"] != "t1" {
		t.Errorf("sessionKey: want t1, got %v", gotBody["sessionKey"])
	}
}

func TestAdapter_ShadowSovereignty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":{"answer":"ok"}}`))
	}))
	defer srv.Close()
	os.Setenv("HC_OPENCLAW_ENDPOINT", srv.URL)
	defer os.Unsetenv("HC_OPENCLAW_ENDPOINT")

	a := &Adapter{}
	out := a.Execute(skills.SkillInput{
		TraceID: "s1",
		Text:    "x",
		Args:    map[string]string{"sovereignty": "shadow"},
	})
	if out.Status != "error" || out.Error != "offline mode" {
		t.Errorf("want offline mode error, got %s: %s", out.Status, out.Error)
	}
}
