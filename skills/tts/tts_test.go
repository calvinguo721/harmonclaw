package tts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"harmonclaw/skills"
)

func TestTTS_Execute_EmptyText(t *testing.T) {
	tx := &TTS{}
	out := tx.Execute(skills.SkillInput{TraceID: "t1", Text: ""})
	if out.Status != "error" {
		t.Errorf("want error, got %s", out.Status)
	}
}

func TestTTS_Execute_Fallback(t *testing.T) {
	os.Unsetenv("HC_TTS_ENDPOINT")

	tx := &TTS{}
	out := tx.Execute(skills.SkillInput{
		TraceID: "t2",
		Text:    "你好。世界！",
		Args:    map[string]string{},
	})
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s: %s", out.Status, out.Error)
	}
	var d map[string]any
	if json.Unmarshal(out.Data, &d) != nil {
		t.Fatal("invalid JSON")
	}
	if d["fallback"] != true {
		t.Error("want fallback true")
	}
	if d["sentences"] == nil {
		t.Error("want sentences")
	}
	if d["phonemes"] == nil {
		t.Error("want phonemes")
	}
}

func TestTTS_Execute_ViaAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake-audio-bytes"))
	}))
	defer srv.Close()

	os.Setenv("HC_TTS_ENDPOINT", srv.URL)
	defer os.Unsetenv("HC_TTS_ENDPOINT")

	tx := &TTS{}
	out := tx.Execute(skills.SkillInput{
		TraceID: "t3",
		Text:    "hello",
		Args:    map[string]string{"sovereignty": "airlock"},
	})
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s: %s", out.Status, out.Error)
	}
	var d map[string]any
	if json.Unmarshal(out.Data, &d) != nil {
		t.Fatal("invalid JSON")
	}
	if d["audio_base64"] == nil {
		t.Error("want audio_base64")
	}
}

func TestSplitToPhonemes(t *testing.T) {
	got := splitToPhonemes("hi 你")
	if len(got) != 3 {
		t.Errorf("want 3 phonemes, got %d: %v", len(got), got)
	}
}
