// Package architect provides per-skill resource quotas (timeout, memory).
package architect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type skillQuota struct {
	TimeoutMs   int `json:"timeout_ms"`
	MaxMemoryMB int `json:"max_memory_mb"`
}

var (
	skillQuotas     map[string]skillQuota
	skillQuotasOnce sync.Once
	defaultTimeout  = 30 * time.Second
)

func loadSkillQuotas(path string) map[string]skillQuota {
	skillQuotasOnce.Do(func() {
		skillQuotas = make(map[string]skillQuota)
		if path == "" {
			path = filepath.Join("configs", "skill-quotas.json")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		var raw map[string]skillQuota
		if json.Unmarshal(data, &raw) == nil {
			skillQuotas = raw
		}
	})
	return skillQuotas
}

// SkillTimeout returns the configured timeout for skillID, or defaultTimeout.
func SkillTimeout(skillID string) time.Duration {
	q := loadSkillQuotas("")
	if s, ok := q[skillID]; ok && s.TimeoutMs > 0 {
		return time.Duration(s.TimeoutMs) * time.Millisecond
	}
	return defaultTimeout
}
