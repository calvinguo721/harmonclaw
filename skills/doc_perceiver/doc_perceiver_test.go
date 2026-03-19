package doc_perceiver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"harmonclaw/skills"
)

func TestPerceiver_Execute_Text(t *testing.T) {
	p := &Perceiver{}
	input := skills.SkillInput{
		TraceID: "test-1",
		Text:    "# Hello\n\nThis is a test document with some keywords like harmonclaw and RISC-V.",
		Args:    map[string]string{},
	}
	out := p.Execute(input)
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s: %s", out.Status, out.Error)
	}
	var d docResult
	if json.Unmarshal(out.Data, &d) != nil {
		t.Fatalf("invalid JSON: %s", string(out.Data))
	}
	if d.Title != "Hello" {
		t.Errorf("title: want Hello, got %s", d.Title)
	}
	if d.Summary == "" {
		t.Error("summary: empty")
	}
	if d.WordCount < 5 {
		t.Errorf("word_count: want >=5, got %d", d.WordCount)
	}
	if d.FileType != "text" {
		t.Errorf("file_type: want text, got %s", d.FileType)
	}
}

func TestPerceiver_Execute_File(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "test.md")
	os.WriteFile(fpath, []byte("# Markdown\n\nContent here."), 0644)

	p := &Perceiver{}
	input := skills.SkillInput{
		TraceID: "test-2",
		Text:    "",
		Args:    map[string]string{"path": fpath},
	}
	out := p.Execute(input)
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s: %s", out.Status, out.Error)
	}
	var d docResult
	if json.Unmarshal(out.Data, &d) != nil {
		t.Fatal("invalid JSON")
	}
	if d.Title != "Markdown" {
		t.Errorf("title: want Markdown, got %s", d.Title)
	}
	if d.FileType != "markdown" {
		t.Errorf("file_type: want markdown, got %s", d.FileType)
	}
}

func TestPerceiver_Execute_Dir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("File A content"), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B\n\nFile B"), 0644)

	p := &Perceiver{}
	input := skills.SkillInput{
		TraceID: "test-3",
		Text:    "",
		Args:    map[string]string{"dir": dir},
	}
	out := p.Execute(input)
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s: %s", out.Status, out.Error)
	}
	var d struct {
		Files []docResult `json:"files"`
	}
	if json.Unmarshal(out.Data, &d) != nil {
		t.Fatal("invalid JSON")
	}
	if len(d.Files) < 2 {
		t.Errorf("files: want >=2, got %d", len(d.Files))
	}
}

func TestPerceiver_Execute_EmptyPathTraversal(t *testing.T) {
	p := &Perceiver{}
	input := skills.SkillInput{
		TraceID: "test-4",
		Text:    "",
		Args:    map[string]string{"path": "../../../etc/passwd"},
	}
	out := p.Execute(input)
	if out.Status != "error" {
		t.Errorf("path traversal: want error, got %s", out.Status)
	}
}

func TestExtractSummary(t *testing.T) {
	short := "Short"
	if s := extractSummary(short); s != "Short" {
		t.Errorf("short: want Short, got %s", s)
	}
	long := "x"
	for i := 0; i < 600; i++ {
		long += "x"
	}
	s := extractSummary(long)
	if len(s) < 500 || len(s) > 510 {
		t.Errorf("long summary len: want ~500, got %d", len(s))
	}
}
