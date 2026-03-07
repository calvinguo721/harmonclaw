// Package configs provides unified configuration with priority chain.
package configs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Config holds runtime configuration. Priority: env > JSON file > defaults.
type Config struct {
	Port            string `json:"port"`
	DataDir         string `json:"data_dir"`
	LogLevel        string `json:"log_level"`
	DeepSeekAPIKey  string `json:"deepseek_api_key"`
	AuthEnabled     bool   `json:"auth_enabled"`
	PoliciesPath   string `json:"policies_path"`
	SovereigntyPath string `json:"sovereignty_path"`
	SovereigntyMode string `json:"sovereignty_mode"`
	Version         string `json:"version"`
}

var (
	instance *Config
	instMu  sync.RWMutex
)

// Get returns the loaded config singleton. Returns nil if not yet loaded.
func Get() *Config {
	instMu.RLock()
	defer instMu.RUnlock()
	return instance
}

// Load reads config: defaults → JSON file (if exists) → env vars. Sets singleton.
func Load(jsonPath string) (*Config, error) {
	loadEnvFromFile(".env")

	c := &Config{
		Port:            "8080",
		DataDir:         defaultDataDir(),
		LogLevel:        "info",
		DeepSeekAPIKey:  os.Getenv("DEEPSEEK_API_KEY"),
		AuthEnabled:     os.Getenv("HC_AUTH_ENABLED") == "true",
		PoliciesPath:    "configs/policies.json",
		SovereigntyPath: "configs/sovereignty.json",
		SovereigntyMode: "airlock",
		Version:         "1.0",
	}

	if jsonPath != "" {
		data, err := os.ReadFile(jsonPath)
		if err == nil {
			var overlay struct {
				Version         string `json:"version"`
				Port            *int   `json:"port"`
				DataDir         string `json:"data_dir"`
				LogLevel        string `json:"log_level"`
				SovereigntyMode string `json:"sovereignty_mode"`
			}
			if json.Unmarshal(data, &overlay) == nil {
				if overlay.Version != "" {
					c.Version = overlay.Version
				}
				if overlay.Port != nil && *overlay.Port > 0 {
					c.Port = strconv.Itoa(*overlay.Port)
				}
				if overlay.DataDir != "" {
					c.DataDir = overlay.DataDir
				}
				if overlay.LogLevel != "" {
					c.LogLevel = overlay.LogLevel
				}
				if overlay.SovereigntyMode != "" {
					c.SovereigntyMode = overlay.SovereigntyMode
				}
			}
		}
	}

	if p := os.Getenv("HC_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			c.Port = p
		}
	}
	if d := os.Getenv("HC_DATA_DIR"); d != "" {
		c.DataDir = d
	}
	if l := os.Getenv("HC_LOG_LEVEL"); l != "" {
		c.LogLevel = l
	}
	if m := os.Getenv("HC_SOVEREIGNTY_MODE"); m != "" {
		c.SovereigntyMode = m
	}
	if p := os.Getenv("HC_IRONCLAW_POLICIES"); p != "" {
		c.PoliciesPath = p
	}
	if k := os.Getenv("DEEPSEEK_API_KEY"); k != "" {
		c.DeepSeekAPIKey = k
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	instMu.Lock()
	instance = c
	instMu.Unlock()
	return c, nil
}

func loadEnvFromFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			k := strings.TrimSpace(line[:idx])
			v := strings.TrimSpace(line[idx+1:])
			v = strings.Trim(v, "\"'")
			if k != "" {
				os.Setenv(k, v)
			}
		}
	}
}

// Validate checks config consistency.
func (c *Config) Validate() error {
	if c.Port == "" {
		return fmt.Errorf("port required")
	}
	if _, err := strconv.Atoi(c.Port); err != nil {
		return fmt.Errorf("invalid port: %s", c.Port)
	}
	if c.DataDir == "" {
		return fmt.Errorf("data_dir required")
	}
	return nil
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".harmonclaw"
	}
	return filepath.Join(home, ".harmonclaw")
}

func (c *Config) VikingEngramsDir() string {
	return filepath.Join(c.DataDir, "viking", "engrams")
}

func (c *Config) LedgerDir() string {
	return filepath.Join(c.DataDir, "ledger")
}

func (c *Config) VikingBaseDir() string {
	return filepath.Join(c.DataDir, "viking")
}

func (c *Config) EnsureDirs() error {
	for _, d := range []string{c.VikingEngramsDir(), c.LedgerDir(), c.VikingBaseDir()} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}
