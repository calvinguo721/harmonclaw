package web_search

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"harmonclaw/skills"
)

func TestSearch_Execute_EmptyQuery(t *testing.T) {
	s := &Search{}
	out := s.Execute(skills.SkillInput{TraceID: "t1", Text: "", Args: map[string]string{}})
	if out.Status != "error" {
		t.Errorf("want error, got %s", out.Status)
	}
}

func TestSearch_Execute_ViaAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[{"title":"A","url":"https://a.com","content":"snippet A"},{"title":"B","url":"https://b.com","content":"snippet B"}]}`))
	}))
	defer srv.Close()

	os.Setenv("HC_SEARCH_API", srv.URL+"?format=json")
	defer os.Unsetenv("HC_SEARCH_API")

	s := &Search{}
	out := s.Execute(skills.SkillInput{
		TraceID: "t2",
		Text:    "test",
		Args:    map[string]string{"sovereignty": "airlock"},
	})
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s: %s", out.Status, out.Error)
	}
	var results []searchResult
	if json.Unmarshal(out.Data, &results) != nil {
		t.Fatal("invalid JSON")
	}
	if len(results) < 2 {
		t.Errorf("want >=2 results, got %d", len(results))
	}
	if results[0].Title != "A" || results[0].URL != "https://a.com" {
		t.Errorf("first result: got %+v", results[0])
	}
}

func TestSearch_Execute_ViaDuckDuckGoHTML(t *testing.T) {
	html := `
	<a class="result__a" href="https://example.com/1">First Result</a>
	<a class="result__snippet">First snippet here</a>
	<a class="result__a" href="https://example.com/2">Second Result</a>
	<a class="result__snippet">Second snippet</a>
	`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	}))
	defer srv.Close()

	os.Unsetenv("HC_SEARCH_API")
	old := duckDuckGoURL
	duckDuckGoURL = srv.URL
	defer func() { duckDuckGoURL = old }()

	s := &Search{}
	out := s.Execute(skills.SkillInput{
		TraceID: "t3",
		Text:    "test",
		Args:    map[string]string{"sovereignty": "airlock"},
	})
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s: %s", out.Status, out.Error)
	}
	var results []searchResult
	if json.Unmarshal(out.Data, &results) != nil {
		t.Fatal("invalid JSON")
	}
	if len(results) < 2 {
		t.Errorf("want >=2 results, got %d", len(results))
	}
}

func TestHtmlUnescape(t *testing.T) {
	if s := htmlUnescape("a&amp;b"); s != "a&b" {
		t.Errorf("want a&b, got %s", s)
	}
}
