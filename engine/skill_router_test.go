package engine

import (
	"testing"

	_ "harmonclaw/skills/doc_perceiver"
	_ "harmonclaw/skills/openclaw_adapter"
	_ "harmonclaw/skills/web_search"
)

func TestSkillRouter_Route(t *testing.T) {
	sr := NewSkillRouter()

	ids := sr.Route("搜索一下天气")
	if len(ids) == 0 {
		t.Error("expected web_search match")
	}
	found := false
	for _, id := range ids {
		if id == "web_search" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("web_search not in %v", ids)
	}
}

func TestSkillRouter_Route_Doc(t *testing.T) {
	sr := NewSkillRouter()

	ids := sr.Route("解析这个文档")
	if len(ids) == 0 {
		t.Error("expected doc_perceiver match")
	}
}

func TestSkillRouter_Route_Empty(t *testing.T) {
	sr := NewSkillRouter()

	if ids := sr.Route(""); len(ids) != 0 {
		t.Errorf("empty: got %v", ids)
	}
	if ids := sr.Route("   "); len(ids) != 0 {
		t.Errorf("blank: got %v", ids)
	}
}

func TestSkillRouter_RouteWithFallback(t *testing.T) {
	sr := NewSkillRouter()

	ids := sr.RouteWithFallback("hello world", "openclaw_proxy")
	if len(ids) != 1 || ids[0] != "openclaw_proxy" {
		t.Errorf("fallback: got %v", ids)
	}
}

func TestSkillRouter_AddKeywords(t *testing.T) {
	sr := NewSkillRouter()
	sr.AddKeywords("doc_perceiver", []string{"custom", "xyz"})

	ids := sr.Route("parse xyz file")
	if len(ids) == 0 {
		t.Error("expected doc_perceiver match after AddKeywords")
	}
}
