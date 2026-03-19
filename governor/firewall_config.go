// Package governor provides firewall configuration loading.
package governor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FirewallConfig holds firewall settings.
type FirewallConfig struct {
	MaxBodyBytes        int      `json:"max_body_bytes"`
	MaxRequestsPerIP    int      `json:"max_requests_per_ip"`
	BanDurationSec      int      `json:"ban_duration_sec"`
	PathBlocklist       []string `json:"path_blocklist"`
	BlockSuspiciousHdrs bool     `json:"block_suspicious_headers"`
}

// DefaultFirewallConfig returns defaults.
func DefaultFirewallConfig() FirewallConfig {
	return FirewallConfig{
		MaxBodyBytes:        1 << 20,
		MaxRequestsPerIP:    20,
		BanDurationSec:      60,
		PathBlocklist:       []string{"../", "..\\", "%2e%2e", "..%2f", "%2e%2e/"},
		BlockSuspiciousHdrs: true,
	}
}

// LoadFirewallConfig loads from configs/governor.json.
func LoadFirewallConfig(path string) FirewallConfig {
	cfg := DefaultFirewallConfig()
	paths := []string{path}
	if path == "" {
		paths = []string{"configs/governor.json"}
		if wd, _ := os.Getwd(); wd != "" {
			paths = append(paths, filepath.Join(wd, "configs/governor.json"))
		}
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var raw struct {
			Firewall *FirewallConfig `json:"firewall"`
		}
		if json.Unmarshal(data, &raw) == nil && raw.Firewall != nil {
			if raw.Firewall.MaxBodyBytes > 0 {
				cfg.MaxBodyBytes = raw.Firewall.MaxBodyBytes
			}
			if raw.Firewall.MaxRequestsPerIP > 0 {
				cfg.MaxRequestsPerIP = raw.Firewall.MaxRequestsPerIP
			}
			if raw.Firewall.BanDurationSec > 0 {
				cfg.BanDurationSec = raw.Firewall.BanDurationSec
			}
			if len(raw.Firewall.PathBlocklist) > 0 {
				cfg.PathBlocklist = raw.Firewall.PathBlocklist
			}
			cfg.BlockSuspiciousHdrs = raw.Firewall.BlockSuspiciousHdrs
			break
		}
	}
	return cfg
}

// BanDuration returns ban duration.
func (c FirewallConfig) BanDuration() time.Duration {
	if c.BanDurationSec <= 0 {
		return 60 * time.Second
	}
	return time.Duration(c.BanDurationSec) * time.Second
}

// ContainsPathTraversal returns true if path contains blocklisted patterns.
func (c FirewallConfig) ContainsPathTraversal(path string) bool {
	pathLower := strings.ToLower(path)
	for _, p := range c.PathBlocklist {
		if strings.Contains(pathLower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}
