// Package governor provides rule-based sovereignty evaluation driven by JSON config.
package governor

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
)

// SovereigntyRule defines a single rule: type, match, action.
type SovereigntyRule struct {
	Type     string `json:"type"`     // "mode", "domain", "port", "scheme"
	Match    string `json:"match"`    // value to match
	Action   string `json:"action"`   // "allow", "deny"
	WhenMode string `json:"when_mode"` // optional: only apply when mode matches
}

// SovereigntyConfig holds rules and legacy modes for backward compatibility.
type SovereigntyConfig struct {
	Version string                     `json:"version"`
	Rules   []SovereigntyRule          `json:"rules"`
	Modes   map[string]struct {
		Desc    string   `json:"description"`
		Domains []string `json:"allowed_domains"`
	} `json:"modes"`
}

var (
	sovereigntyRules   []SovereigntyRule
	sovereigntyRulesMu sync.RWMutex
)

// LoadSovereigntyConfig loads and parses sovereignty.json. Supports both rules (v2) and modes (v1) formats.
// Returns mode, domains for backward compat with InitSecureClient.
func LoadSovereigntyConfig(path, mode string) (string, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return mode, nil, err
	}
	var cfg SovereigntyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return mode, nil, err
	}
	domains := []string{}

	// New format: rules array
	if len(cfg.Rules) > 0 {
		sovereigntyRulesMu.Lock()
		sovereigntyRules = cfg.Rules
		sovereigntyRulesMu.Unlock()
		// Derive domains from mode for backward compat (e.g. API responses)
		if cfg.Modes != nil {
			if m, ok := cfg.Modes[mode]; ok {
				domains = m.Domains
			}
		}
		return mode, domains, nil
	}

	// Legacy format: modes only, clear rules so client uses domainAllowedLocked
	sovereigntyRulesMu.Lock()
	sovereigntyRules = nil
	sovereigntyRulesMu.Unlock()
	if cfg.Modes != nil {
		if m, ok := cfg.Modes[mode]; ok {
			domains = m.Domains
		}
	}
	return mode, domains, nil
}

// EvaluateRules checks if (host, portStr, scheme) is allowed under the given mode.
// Returns true if allowed, false if denied. Uses rules when loaded; else returns false (safe default).
func EvaluateRules(host, portStr, scheme, mode string) bool {
	sovereigntyRulesMu.RLock()
	rules := sovereigntyRules
	sovereigntyRulesMu.RUnlock()

	if len(rules) == 0 {
		return false
	}

	hostOnly := host
	if idx := strings.Index(host, ":"); idx >= 0 {
		hostOnly = host[:idx]
	}
	port := 0
	if portStr != "" {
		port, _ = strconv.Atoi(portStr)
	}
	if scheme == "https" && port == 0 {
		port = 443
	}
	if scheme == "http" && port == 0 {
		port = 80
	}
	scheme = strings.ToLower(scheme)

	// First pass: deny rules take precedence
	for _, r := range rules {
		if r.WhenMode != "" && r.WhenMode != mode {
			continue
		}
		if r.Action != "deny" {
			continue
		}
		if ruleMatches(r, hostOnly, port, scheme, mode) {
			return false
		}
	}

	// Second pass: allow rules
	for _, r := range rules {
		if r.WhenMode != "" && r.WhenMode != mode {
			continue
		}
		if r.Action != "allow" {
			continue
		}
		if ruleMatches(r, hostOnly, port, scheme, mode) {
			return true
		}
	}

	return false
}

func ruleMatches(r SovereigntyRule, host string, port int, scheme, mode string) bool {
	switch r.Type {
	case "mode":
		return r.Match == mode
	case "domain":
		if r.Match == "*" {
			return true
		}
		if r.Match == host {
			return true
		}
		if strings.HasPrefix(r.Match, "*.") {
			suffix := r.Match[1:]
			return host == suffix || strings.HasSuffix(host, suffix)
		}
		return false
	case "port":
		p, _ := strconv.Atoi(r.Match)
		return p == port
	case "scheme":
		return strings.ToLower(r.Match) == scheme
	}
	return false
}

// GetSovereigntyRules returns the current rules (for testing).
func GetSovereigntyRules() []SovereigntyRule {
	sovereigntyRulesMu.RLock()
	defer sovereigntyRulesMu.RUnlock()
	out := make([]SovereigntyRule, len(sovereigntyRules))
	copy(out, sovereigntyRules)
	return out
}
