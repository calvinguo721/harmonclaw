package ironclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"harmonclaw/governor"
)

// Policy 安全策略
type Policy struct {
	SkillID          string   `json:"skill_id"`
	AllowedUsers     []string `json:"allowed_users"`
	MaxQPS           int      `json:"max_qps"`
	RequireToken     bool     `json:"require_token"`
	MinClassification string `json:"min_classification"`
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
