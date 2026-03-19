package governor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateRules_ShadowDeny(t *testing.T) {
	// Set rules directly for testing (bypass LoadSovereigntyConfig)
	sovereigntyRulesMu.Lock()
	sovereigntyRules = []SovereigntyRule{
		{Type: "mode", Match: "shadow", Action: "deny"},
		{Type: "domain", Match: "*", Action: "allow", WhenMode: "opensea"},
		{Type: "domain", Match: "api.deepseek.com", Action: "allow"},
	}
	sovereigntyRulesMu.Unlock()
	defer func() {
		sovereigntyRulesMu.Lock()
		sovereigntyRules = nil
		sovereigntyRulesMu.Unlock()
	}()

	if EvaluateRules("api.deepseek.com", "443", "https", "shadow") {
		t.Error("shadow mode should deny all")
	}
	if EvaluateRules("any.com", "443", "https", "shadow") {
		t.Error("shadow mode should deny all")
	}
}

func TestEvaluateRules_AirlockWhitelist(t *testing.T) {
	sovereigntyRulesMu.Lock()
	sovereigntyRules = []SovereigntyRule{
		{Type: "mode", Match: "shadow", Action: "deny"},
		{Type: "domain", Match: "*", Action: "allow", WhenMode: "opensea"},
		{Type: "domain", Match: "api.deepseek.com", Action: "allow"},
		{Type: "domain", Match: "localhost", Action: "allow"},
	}
	sovereigntyRulesMu.Unlock()
	defer func() {
		sovereigntyRulesMu.Lock()
		sovereigntyRules = nil
		sovereigntyRulesMu.Unlock()
	}()

	if !EvaluateRules("api.deepseek.com", "443", "https", "airlock") {
		t.Error("airlock should allow whitelisted api.deepseek.com")
	}
	if !EvaluateRules("localhost", "8080", "http", "airlock") {
		t.Error("airlock should allow localhost")
	}
	if EvaluateRules("evil.com", "443", "https", "airlock") {
		t.Error("airlock should deny non-whitelisted")
	}
}

func TestEvaluateRules_OpenSeaAllowAll(t *testing.T) {
	sovereigntyRulesMu.Lock()
	sovereigntyRules = []SovereigntyRule{
		{Type: "mode", Match: "shadow", Action: "deny"},
		{Type: "domain", Match: "*", Action: "allow", WhenMode: "opensea"},
	}
	sovereigntyRulesMu.Unlock()
	defer func() {
		sovereigntyRulesMu.Lock()
		sovereigntyRules = nil
		sovereigntyRulesMu.Unlock()
	}()

	if !EvaluateRules("any.com", "443", "https", "opensea") {
		t.Error("opensea with * should allow all")
	}
}

func TestLoadSovereigntyConfig_RulesFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sov.json")
	os.WriteFile(path, []byte(`{"version":"2.0","rules":[{"type":"mode","match":"shadow","action":"deny"}]}`), 0644)

	mode, domains, err := LoadSovereigntyConfig(path, "airlock")
	if err != nil {
		t.Fatal(err)
	}
	if mode != "airlock" {
		t.Errorf("mode: want airlock, got %s", mode)
	}
	if len(GetSovereigntyRules()) != 1 {
		t.Errorf("rules: want 1, got %d", len(GetSovereigntyRules()))
	}
	_ = domains
}

func TestLoadSovereigntyConfig_LegacyModesFormat(t *testing.T) {
	sovereigntyRulesMu.Lock()
	sovereigntyRules = nil
	sovereigntyRulesMu.Unlock()

	dir := t.TempDir()
	path := filepath.Join(dir, "sov.json")
	os.WriteFile(path, []byte(`{"version":"1.0","modes":{"airlock":{"allowed_domains":["api.example.com"]}}}`), 0644)

	mode, domains, err := LoadSovereigntyConfig(path, "airlock")
	if err != nil {
		t.Fatal(err)
	}
	if mode != "airlock" {
		t.Errorf("mode: want airlock, got %s", mode)
	}
	if len(domains) != 1 || domains[0] != "api.example.com" {
		t.Errorf("domains: want [api.example.com], got %v", domains)
	}
	// Legacy format: no rules loaded
	if len(GetSovereigntyRules()) != 0 {
		t.Errorf("legacy format should not set rules, got %d", len(GetSovereigntyRules()))
	}
}
