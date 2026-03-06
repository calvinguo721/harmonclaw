// Package architect (registry) provides skill registry with version and health check.
package architect

import (
	"sync"
	"time"

	"harmonclaw/skills"
)

// SkillMeta holds metadata for a registered skill.
type SkillMeta struct {
	ID        string
	Version   string
	Core      string
	Healthy   bool
	LastCheck time.Time
}

// SkillRegistry wraps skills.Registry with version and health.
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

// Get returns skill by ID.
func (r *SkillRegistry) Get(id string) (skills.Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[id]
	return s, ok
}

// List returns all skill IDs.
func (r *SkillRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.skills))
	for id := range r.skills {
		ids = append(ids, id)
	}
	return ids
}

// Meta returns metadata for a skill.
func (r *SkillRegistry) Meta(id string) (SkillMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.meta[id]
	return m, ok
}

// HealthCheck marks skill healthy/unhealthy.
func (r *SkillRegistry) HealthCheck(id string, healthy bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := r.meta[id]; ok {
		m.Healthy = healthy
		m.LastCheck = time.Now()
		r.meta[id] = m
	}
}

// SyncFromGlobal copies from skills.Registry (for hot-load compatibility).
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
