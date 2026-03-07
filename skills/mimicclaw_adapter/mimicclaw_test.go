package mimicclaw_adapter

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
		w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	os.Setenv("HC_MIMICCLAW_ENDPOINT", srv.URL)
	defer os.Unsetenv("HC_MIMICCLAW_ENDPOINT")

	a := &Adapter{}
	out := a.Execute(skills.SkillInput{TraceID: "t1", Text: "hi"})
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s", out.Status)
	}
	var d map[string]any
	json.Unmarshal(out.Data, &d)
	if d["degraded"] == true {
		t.Error("unexpected degraded")
	}
}

func TestAdapter_ShadowSovereignty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()
	os.Setenv("HC_MIMICCLAW_ENDPOINT", srv.URL)
	defer os.Unsetenv("HC_MIMICCLAW_ENDPOINT")

	a := &Adapter{}
	out := a.Execute(skills.SkillInput{TraceID: "s1", Text: "x", Args: map[string]string{"sovereignty": "shadow"}})
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s", out.Status)
	}
	var d map[string]any
	json.Unmarshal(out.Data, &d)
	if d["degraded"] != true || d["reason"] != "offline mode" {
		t.Errorf("want degraded offline, got %v", d)
	}
}

func TestAdapter_Execute_Degraded(t *testing.T) {
	os.Setenv("HC_MIMICCLAW_ENDPOINT", "http://127.0.0.1:19998")
	defer os.Unsetenv("HC_MIMICCLAW_ENDPOINT")

	a := &Adapter{}
	out := a.Execute(skills.SkillInput{TraceID: "t2", Text: "hi"})
	if out.Status != "ok" {
		t.Fatalf("want ok (graceful), got %s", out.Status)
	}
	var d map[string]any
	json.Unmarshal(out.Data, &d)
	if d["degraded"] != true {
		t.Error("want degraded true")
	}
}
