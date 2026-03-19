// Package architect (registry) provides runtime register/unregister, 30s health check, 3 failed=degraded.
package architect

import (
	"sort"
	"sync"
	"time"

	"harmonclaw/skills"
)

const (
	healthCheckInterval = 30 * time.Second
	degradedFailCount   = 3
)

// SkillMeta holds metadata for a registered skill.
type SkillMeta struct {
	ID         string
	Version    string
	Core       string
	Healthy    bool
	LastCheck  time.Time
	FailCount  int
}

// SkillRegistry wraps skills with runtime register/unregister and health check.
type SkillRegistry struct {
	mu     sync.RWMutex
	meta   map[string]SkillMeta
	skills map[string]skills.Skill
}

// NewSkillRegistry creates a registry.
func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		meta:   make(map[string]SkillMeta),
		skills: make(map[string]skills.Skill),
	}
}

// Register adds a skill with metadata.
func (r *SkillRegistry) Register(skill skills.Skill) {
	id := skill.GetIdentity()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[id.ID] = skill
	r.meta[id.ID] = SkillMeta{
		ID:      id.ID,
		Version: id.Version,
		Core:    id.Core,
		Healthy: true,
	}
}

// Unregister removes a skill.
func (r *SkillRegistry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.skills, id)
	delete(r.meta, id)
}

// Get returns skill by ID.
func (r *SkillRegistry) Get(id string) (skills.Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[id]
	return s, ok
}

// List returns all skill IDs (sorted).
func (r *SkillRegistry) List() []string {
	r.mu.RLock()
	ids := make([]string, 0, len(r.skills))
	for id := range r.skills {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	sort.Strings(ids)
	return ids
}

// Meta returns metadata for a skill.
func (r *SkillRegistry) Meta(id string) (SkillMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.meta[id]
	return m, ok
}

// HealthCheck marks skill healthy/unhealthy. 3 failed = degraded (Healthy=false).
func (r *SkillRegistry) HealthCheck(id string, healthy bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := r.meta[id]; ok {
		m.LastCheck = time.Now()
		if healthy {
			m.FailCount = 0
			m.Healthy = true
		} else {
			m.FailCount++
			if m.FailCount >= degradedFailCount {
				m.Healthy = false
			}
		}
		r.meta[id] = m
	}
}

// StartHealthCheck runs 30s health check loop. checkFn(id) returns healthy.
func (r *SkillRegistry) StartHealthCheck(checkFn func(string) bool) {
	go func() {
		ticker := time.NewTicker(healthCheckInterval)
		defer ticker.Stop()
		for range ticker.C {
			for _, id := range r.List() {
				healthy := checkFn(id)
				r.HealthCheck(id, healthy)
			}
		}
	}()
}

// SyncFromGlobal copies from skills.Registry.
func (r *SkillRegistry) SyncFromGlobal() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, s := range skills.Registry {
		r.skills[id] = s
		meta := r.meta[id]
		meta.ID = id
		meta.Version = s.GetIdentity().Version
		meta.Core = s.GetIdentity().Core
		if meta.LastCheck.IsZero() {
			meta.Healthy = true
		}
		r.meta[id] = meta
	}
}
