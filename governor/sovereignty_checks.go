// Package governor provides GeoIP and hardware whitelist checks for sovereignty.
package governor

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type allowedBoardsConfig struct {
	Boards []struct {
		Name   string `json:"name"`
		Arch   string `json:"arch"`
		Status string `json:"status"`
	} `json:"boards"`
	OS  []string `json:"os"`
	Geo []string `json:"geo"`
}

var (
	boardsConfig     *allowedBoardsConfig
	boardsConfigOnce sync.Once
)

func loadAllowedBoards(path string) *allowedBoardsConfig {
	boardsConfigOnce.Do(func() {
		if path == "" {
			path = filepath.Join("configs", "allowed-boards.json")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		var c allowedBoardsConfig
		if json.Unmarshal(data, &c) == nil {
			boardsConfig = &c
		}
	})
	return boardsConfig
}

// GeoIPCheck returns true if IP is "domestic" (placeholder: private IPs + localhost).
func GeoIPCheck(ip string) bool {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	if parsed.IsLoopback() || parsed.IsPrivate() {
		return true
	}
	return false
}

// AllowedBoardsCheck returns true if current arch is in whitelist (placeholder).
func AllowedBoardsCheck(arch string) bool {
	cfg := loadAllowedBoards("")
	if cfg == nil {
		return true
	}
	for _, b := range cfg.Boards {
		if b.Status == "approved" && b.Arch == arch {
			return true
		}
	}
	return false
}
