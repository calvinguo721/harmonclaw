package web_search

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestSearch_CacheHit(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[{"title":"Cached","url":"https://cached.com","content":"from cache"}]}`))
	}))
	defer srv.Close()

	os.Setenv("HC_SEARCH_API", srv.URL+"?format=json")
	defer os.Unsetenv("HC_SEARCH_API")

	s := &Search{}
	q := skills.SkillInput{TraceID: "c1", Text: "cache test", Args: map[string]string{"sovereignty": "airlock"}}

	out1 := s.Execute(q)
	if out1.Status != "ok" {
		t.Fatalf("first: want ok, got %s", out1.Status)
	}
	out2 := s.Execute(q)
	if out2.Status != "ok" {
		t.Fatalf("second: want ok, got %s", out2.Status)
	}
	if callCount != 1 {
		t.Errorf("want 1 API call (cache hit on 2nd), got %d", callCount)
	}
}

func TestSearch_ViaSearXNG(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("want /search, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[{"title":"SearX","url":"https://sx.com","content":"SearXNG result"}]}`))
	}))
	defer srv.Close()

	os.Unsetenv("HC_SEARCH_API")
	os.Setenv("HC_SEARCH_SEARXNG", srv.URL)
	defer os.Unsetenv("HC_SEARCH_SEARXNG")

	s := &Search{}
	out := s.Execute(skills.SkillInput{
		TraceID: "sx1",
		Text:    "searxng query",
		Args:    map[string]string{"sovereignty": "airlock"},
	})
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s: %s", out.Status, out.Error)
	}
	var results []searchResult
	if json.Unmarshal(out.Data, &results) != nil {
		t.Fatal("invalid JSON")
	}
	if len(results) != 1 || results[0].Title != "SearX" || results[0].Snippet != "SearXNG result" {
		t.Errorf("got %+v", results)
	}
}

func TestSearch_ConcurrencyLimit(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
		w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	defer close(block)

	os.Setenv("HC_SEARCH_API", srv.URL+"?format=json")
	defer os.Unsetenv("HC_SEARCH_API")

	s := &Search{}
	acquireSearchSlot()
	acquireSearchSlot()
	acquireSearchSlot()
	defer func() {
		releaseSearchSlot()
		releaseSearchSlot()
		releaseSearchSlot()
	}()

	out := s.Execute(skills.SkillInput{TraceID: "cl1", Text: "blocked", Args: map[string]string{"sovereignty": "airlock"}})
	if out.Status != "error" || !strings.Contains(out.Error, "concurrency") {
		t.Errorf("want concurrency error, got %s: %s", out.Status, out.Error)
	}
}
