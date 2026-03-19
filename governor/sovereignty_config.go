// Package governor provides three-tier sovereignty: personal (zero network), local (LAN only), connected (whitelist + audit).
package governor

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// SovereigntyMode is the three-tier mode.
type SovereigntyMode string

const (
	ModePersonal  SovereigntyMode = "personal"
	ModeLocal     SovereigntyMode = "local"
	ModeConnected SovereigntyMode = "connected"
)

// SovereigntyConfig holds the three-tier config.
type SovereigntyConfig struct {
	Mode      string          `json:"mode"`
	Personal  PersonalConfig  `json:"personal"`
	Local     LocalConfig     `json:"local"`
	Connected ConnectedConfig `json:"connected"`
}

// PersonalConfig is zero network.
type PersonalConfig struct {
	Network         string   `json:"network"`
	AllowedEndpoints []string `json:"allowed_endpoints"`
}

// LocalConfig is LAN-only.
type LocalConfig struct {
	Network         string   `json:"network"`
	AllowedSubnets  []string `json:"allowed_subnets"`
	AllowedEndpoints []string `json:"allowed_endpoints"`
}

// ConnectedConfig is filtered with whitelist and audit.
type ConnectedConfig struct {
	Network   string   `json:"network"`
	Whitelist []string `json:"whitelist"`
	Audit     bool     `json:"audit"`
	Ledger    bool     `json:"ledger"`
}

var (
	sovConfig   *SovereigntyConfig
	sovConfigMu sync.RWMutex
)

// LoadSovereigntyConfig reads configs/sovereignty.json. Supports new (mode/personal/local/connected) and legacy (modes) formats.
func LoadSovereigntyConfig(path string) (*SovereigntyConfig, error) {
	if path == "" {
		path = "configs/sovereignty.json"
	}
	paths := []string{path}
	if wd, _ := os.Getwd(); wd != "" {
		paths = append(paths, filepath.Join(wd, path))
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var raw map[string]json.RawMessage
		if json.Unmarshal(data, &raw) != nil {
			continue
		}
		if _, hasModes := raw["modes"]; hasModes {
			// Legacy format: {version, modes: {shadow: {allowed_domains}, airlock: {...}, opensea: {...}}}
			var legacy struct {
				Modes map[string]struct {
					Domains []string `json:"allowed_domains"`
				} `json:"modes"`
			}
			if json.Unmarshal(data, &legacy) != nil {
				continue
			}
			cfg := defaultSovereigntyConfig()
			if m, ok := legacy.Modes["opensea"]; ok && len(m.Domains) > 0 {
				cfg.Mode = "opensea"
				cfg.Connected.Whitelist = m.Domains
			} else if m, ok := legacy.Modes["airlock"]; ok && len(m.Domains) > 0 {
				cfg.Mode = "airlock"
				cfg.Connected.Whitelist = m.Domains
			} else {
				cfg.Mode = "shadow"
			}
			sovConfigMu.Lock()
			sovConfig = cfg
			sovConfigMu.Unlock()
			return cfg, nil
		}
		var cfg SovereigntyConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
		if cfg.Mode == "" {
			cfg.Mode = string(ModePersonal)
		}
		if len(cfg.Local.AllowedSubnets) == 0 {
			cfg.Local.AllowedSubnets = []string{"192.168.0.0/16", "10.0.0.0/8"}
		}
		if len(cfg.Connected.Whitelist) == 0 {
			cfg.Connected.Whitelist = []string{"api.deepseek.com", "api.openai.com"}
		}
		sovConfigMu.Lock()
		sovConfig = &cfg
		sovConfigMu.Unlock()
		return &cfg, nil
	}
	cfg := defaultSovereigntyConfig()
	sovConfigMu.Lock()
	sovConfig = cfg
	sovConfigMu.Unlock()
	return cfg, nil
}

func defaultSovereigntyConfig() *SovereigntyConfig {
	return &SovereigntyConfig{
		Mode:     string(ModePersonal),
		Personal: PersonalConfig{Network: "blocked"},
		Local: LocalConfig{
			Network:        "lan_only",
			AllowedSubnets: []string{"192.168.0.0/16", "10.0.0.0/8"},
		},
		Connected: ConnectedConfig{
			Network:   "filtered",
			Whitelist: []string{"api.deepseek.com", "api.openai.com"},
			Audit:     true,
			Ledger:    true,
		},
	}
}

// GetSovereigntyConfig returns the loaded config.
func GetSovereigntyConfig() *SovereigntyConfig {
	sovConfigMu.RLock()
	defer sovConfigMu.RUnlock()
	if sovConfig == nil {
		return defaultSovereigntyConfig()
	}
	return sovConfig
}

// SetSovereigntyConfig updates mode and whitelist at runtime (for API POST).
func SetSovereigntyConfig(mode string, whitelist []string) {
	sovConfigMu.Lock()
	defer sovConfigMu.Unlock()
	if sovConfig == nil {
		sovConfig = defaultSovereigntyConfig()
	}
	sovConfig.Mode = mode
	if whitelist != nil {
		sovConfig.Connected.Whitelist = whitelist
	}
}

// ResolveMode maps legacy modes to new ones.
func ResolveMode(mode string) string {
	switch mode {
	case "shadow":
		return string(ModePersonal)
	case "airlock", "opensea":
		return string(ModeConnected)
	}
	return mode
}

// IsAllowed returns true if target host is allowed for current mode.
func (sc *SovereigntyConfig) IsAllowed(target string) bool {
	mode := ResolveMode(sc.Mode)
	switch mode {
	case string(ModePersonal):
		return false
	case string(ModeLocal):
		return isLAN(target, sc.Local.AllowedSubnets)
	case string(ModeConnected):
		return isWhitelisted(target, sc.Connected.Whitelist)
	}
	return false
}

func isLAN(host string, subnets []string) bool {
	hostOnly := host
	if idx := strings.Index(host, ":"); idx >= 0 {
		hostOnly = host[:idx]
	}
	ips, err := net.LookupIP(hostOnly)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		ip4 := ip.To4()
		if ip4 == nil {
			continue
		}
		for _, cidr := range subnets {
			_, net, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			if net.Contains(ip) {
				return true
			}
		}
	}
	return false
}

func isWhitelisted(host string, whitelist []string) bool {
	hostOnly := host
	if idx := strings.Index(host, ":"); idx >= 0 {
		hostOnly = host[:idx]
	}
	for _, w := range whitelist {
		if w == "*" {
			return true
		}
		if w == hostOnly {
			return true
		}
		if strings.HasPrefix(w, "*.") {
			suffix := w[1:]
			if hostOnly == suffix || strings.HasSuffix(hostOnly, suffix) {
				return true
			}
		}
	}
	return false
}
