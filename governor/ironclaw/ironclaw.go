// Package ironclaw provides security policy matrix and enforcement.
package ironclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"harmonclaw/governor"
)

// Policy 安全策略
type Policy struct {
	SkillID           string   `json:"skill_id"`
	AllowedUsers      []string `json:"allowed_users"`
	MaxQPS            int      `json:"max_qps"`
	RequireToken      bool     `json:"require_token"`
	MinClassification string   `json:"min_classification"`
}

// PathRule 路径级策略
type PathRule struct {
	Path        string   `json:"path"`
	Methods     []string `json:"methods"`
	RequireAuth bool     `json:"require_auth"`
}

// RulesMatrix 安全矩阵配置
type RulesMatrix struct {
	PathRules    []PathRule `json:"path_rules"`
	BlockedPaths []string   `json:"blocked_paths"`
	DefaultAllow bool       `json:"default_allow"`
}

// LoadRulesMatrix loads from configs/ironclaw_rules.json.
func LoadRulesMatrix(path string) RulesMatrix {
	rm := RulesMatrix{DefaultAllow: true}
	paths := []string{path}
	if path == "" {
		paths = []string{"configs/ironclaw_rules.json"}
		if wd, _ := os.Getwd(); wd != "" {
			paths = append(paths, filepath.Join(wd, "configs/ironclaw_rules.json"))
		}
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		json.Unmarshal(data, &rm)
		break
	}
	return rm
}

// CheckPath returns nil if path+method allowed, error if blocked.
func (r RulesMatrix) CheckPath(path, method string) error {
	for _, blocked := range r.BlockedPaths {
		if strings.HasPrefix(path, blocked) || strings.Contains(path, blocked) {
			return fmt.Errorf("ironclaw: path %s blocked", path)
		}
	}
	for _, rule := range r.PathRules {
		if strings.HasPrefix(path, rule.Path) {
			if len(rule.Methods) == 0 {
				return nil
			}
			for _, m := range rule.Methods {
				if strings.EqualFold(m, method) {
					return nil
				}
			}
			return fmt.Errorf("ironclaw: method %s not allowed for %s", method, path)
		}
	}
	if !r.DefaultAllow {
		return fmt.Errorf("ironclaw: path %s not in allowlist", path)
	}
	return nil
}

// Request 策略检查请求
type Request struct {
	UserID         string
	SkillID        string
	Token          string
	Classification string
}

var (
	qpsMu   sync.Mutex
	qpsHits = make(map[string][]int64)
)

var classificationOrder = map[string]int{
	"public": 0, "internal": 1, "confidential": 2, "secret": 3,
}

func classificationLevel(s string) int {
	if v, ok := classificationOrder[strings.ToLower(s)]; ok {
		return v
	}
	return 0
}

// LoadPolicies 从 JSON 文件加载策略
func LoadPolicies(path string) ([]Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Policy{}, nil
		}
		return nil, fmt.Errorf("load policies: %w", err)
	}
	var policies []Policy
	if err := json.Unmarshal(data, &policies); err != nil {
		return nil, fmt.Errorf("parse policies: %w", err)
	}
	return policies, nil
}

// Enforce 检查策略：用户权限 + QPS + Token + 数据分级
func Enforce(policy Policy, req Request) error {
	if policy.SkillID == "" {
		return nil
	}

	if len(policy.AllowedUsers) > 0 {
		allowed := false
		for _, u := range policy.AllowedUsers {
			if u == req.UserID {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("ironclaw: user %s not in allowed_users", req.UserID)
		}
	}

	if policy.MaxQPS > 0 {
		qpsMu.Lock()
		now := time.Now().Unix()
		cutoff := now - 1
		times := qpsHits[policy.SkillID]
		j := 0
		for _, t := range times {
			if t > cutoff {
				times[j] = t
				j++
			}
		}
		times = times[:j]
		if len(times) >= policy.MaxQPS {
			qpsMu.Unlock()
			return fmt.Errorf("ironclaw: skill %s QPS exceeded (%d)", policy.SkillID, policy.MaxQPS)
		}
		qpsHits[policy.SkillID] = append(times, now)
		qpsMu.Unlock()
	}

	if policy.RequireToken {
		if req.Token == "" {
			return fmt.Errorf("ironclaw: token required for skill %s", policy.SkillID)
		}
		if _, err := governor.ValidateToken(req.Token); err != nil {
			return fmt.Errorf("ironclaw: invalid token: %w", err)
		}
	}

	if policy.MinClassification != "" {
		minLev := classificationLevel(policy.MinClassification)
		reqLev := classificationLevel(req.Classification)
		if reqLev < minLev {
			return fmt.Errorf("ironclaw: classification %s below min %s", req.Classification, policy.MinClassification)
		}
	}

	return nil
}
