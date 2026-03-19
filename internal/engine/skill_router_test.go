package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillRouter_Route_Single(t *testing.T) {
	sr := NewSkillRouter()
	ids := sr.Route("搜索 Go 语言")
	if len(ids) == 0 {
		t.Fatal("expected at least one skill")
	}
	if ids[0] != "web_search" {
		t.Errorf("first=%q, want web_search", ids[0])
	}
}

func TestSkillRouter_Route_Doc(t *testing.T) {
	sr := NewSkillRouter()
	ids := sr.Route("解析这个文档并总结")
	if len(ids) == 0 {
		t.Fatal("expected doc_perceiver")
	}
	if ids[0] != "doc_perceiver" {
		t.Errorf("first=%q, want doc_perceiver", ids[0])
	}
}

func TestSkillRouter_Route_Empty(t *testing.T) {
	sr := NewSkillRouter()
	ids := sr.Route("")
	if ids != nil {
		t.Errorf("expected nil, got %v", ids)
	}
}

func TestSkillRouter_Route_Chain(t *testing.T) {
	sr := NewSkillRouter()
	ids := sr.Route("搜索 Go 语言 然后 总结")
	if len(ids) < 2 {
		t.Errorf("expected chain (search+summary), got %v", ids)
	}
	hasSearch := false
	hasDoc := false
	for _, id := range ids {
		if id == "web_search" {
			hasSearch = true
		}
		if id == "doc_perceiver" {
			hasDoc = true
		}
	}
	if !hasSearch {
		t.Error("chain should include web_search")
	}
	if !hasDoc {
		t.Error("chain should include doc_perceiver (总结)")
	}
}

func TestSkillRouter_Route_MultiMatch(t *testing.T) {
	sr := NewSkillRouter()
	ids := sr.Route("搜索文档")
	if len(ids) == 0 {
		t.Fatal("expected matches")
	}
	if ids[0] != "web_search" && ids[0] != "doc_perceiver" {
		t.Errorf("first=%q", ids[0])
	}
}

func TestSkillRouter_Route_Priority(t *testing.T) {
	sr := NewSkillRouter()
	sr.AddSkill(SkillEntry{ID: "low_pri", Keywords: []string{"test"}, Priority: 1})
	sr.AddSkill(SkillEntry{ID: "high_pri", Keywords: []string{"test"}, Priority: 100})
	ids := sr.Route("test")
	if len(ids) < 2 {
		t.Fatalf("expected 2, got %v", ids)
	}
	if ids[0] != "high_pri" {
		t.Errorf("higher priority first: got %q", ids[0])
	}
}

func TestSkillRouter_RouteWithFallback(t *testing.T) {
	sr := NewSkillRouter()
	ids := sr.Route("xyz unknown query")
	if len(ids) != 0 {
		t.Errorf("expected no match, got %v", ids)
	}
}

func TestSkillRouter_AddSkill(t *testing.T) {
	sr := NewSkillRouter()
	sr.AddSkill(SkillEntry{ID: "custom", Keywords: []string{"custom", "自定义"}, Priority: 90})
	ids := sr.Route("自定义技能")
	if len(ids) == 0 {
		t.Fatal("expected custom skill")
	}
	if ids[0] != "custom" {
		t.Errorf("first=%q, want custom", ids[0])
	}
}

func TestSkillRouter_FromConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := SkillRouterConfig{
		Skills: []SkillEntry{
			{ID: "from_config", Keywords: []string{"配置", "config"}, Priority: 99},
		},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	path := filepath.Join(dir, "skills.json")
	os.WriteFile(path, data, 0644)

	sr := NewSkillRouterFromConfig(path)
	ids := sr.Route("配置测试")
	if len(ids) == 0 {
		t.Fatal("expected from_config")
	}
	if ids[0] != "from_config" {
		t.Errorf("first=%q, want from_config", ids[0])
	}
}

func TestSkillRouter_ImplementsInterface(t *testing.T) {
	var _ SkillRouter = (*SkillRouterImpl)(nil)
	var _ SkillRouter = NewSkillRouter()
}
