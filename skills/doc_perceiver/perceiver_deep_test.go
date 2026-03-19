package doc_perceiver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"harmonclaw/skills"
)

func TestPerceiver_HTML(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "page.html")
	html := `<!DOCTYPE html><html><head><title>Test</title></head><body><nav>skip</nav><script>skip</script><style>skip</style><p>Main content here. Second sentence. Third one.</p></body></html>`
	os.WriteFile(fpath, []byte(html), 0644)

	p := &Perceiver{}
	out := p.Execute(skills.SkillInput{TraceID: "t1", Args: map[string]string{"path": fpath}})
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s", out.Status)
	}
	var d docResult
	json.Unmarshal(out.Data, &d)
	if d.FileType != "html" {
		t.Errorf("file_type: want html, got %s", d.FileType)
	}
	if d.FileHash == "" {
		t.Error("file_hash: empty")
	}
	if d.FileSize == 0 {
		t.Error("file_size: 0")
	}
}

func TestPerceiver_CSV(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "data.csv")
	os.WriteFile(fpath, []byte("a,b,c\n1,2,3\nx,y,z"), 0644)

	p := &Perceiver{}
	out := p.Execute(skills.SkillInput{TraceID: "t2", Args: map[string]string{"path": fpath}})
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s", out.Status)
	}
	var d docResult
	json.Unmarshal(out.Data, &d)
	if d.FileType != "csv" {
		t.Errorf("file_type: want csv, got %s", d.FileType)
	}
}

func TestPerceiver_LargeFile(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "big.txt")
	data := make([]byte, 2*1024*1024)
	for i := range data {
		data[i] = 'x'
	}
	os.WriteFile(fpath, data, 0644)

	p := &Perceiver{}
	out := p.Execute(skills.SkillInput{TraceID: "t3", Args: map[string]string{"path": fpath}})
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s", out.Status)
	}
	var d docResult
	json.Unmarshal(out.Data, &d)
	if d.FileSize > maxFileBytes+1000 {
		t.Errorf("file_size should be capped, got %d", d.FileSize)
	}
}

func TestPerceiver_RecursiveDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("A"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("B"), 0644)

	p := &Perceiver{}
	out := p.Execute(skills.SkillInput{TraceID: "t4", Args: map[string]string{"dir": dir}})
	if out.Status != "ok" {
		t.Fatalf("want ok, got %s", out.Status)
	}
	var d struct {
		Files []docResult `json:"files"`
	}
	json.Unmarshal(out.Data, &d)
	if len(d.Files) < 2 {
		t.Errorf("recursive: want >=2 files, got %d", len(d.Files))
	}
}

func TestExtractEntities(t *testing.T) {
	ent := extractEntities("Meeting on 2024-03-15 with John Smith, budget 1000000")
	if len(ent) < 2 {
		t.Errorf("expected entities, got %v", ent)
	}
}
