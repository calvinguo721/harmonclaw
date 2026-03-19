// Package butler (persona) loads and manages persona from configs/persona.json.
package butler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// PersonaConfig holds a single persona.
type PersonaConfig struct {
	SystemPrompt string  `json:"system_prompt"`
	Temperature  float64 `json:"temperature"`
	TopP         float64 `json:"top_p"`
}

// PersonaStore loads from configs/persona.json and supports runtime updates.
type PersonaStore struct {
	mu             sync.RWMutex
	path           string
	Personas       map[string]PersonaConfig `json:"personas"`
	DefaultPersona string                   `json:"default_persona"`
}

// NewPersonaStore loads from path. If path is empty, uses configs/persona.json.
func NewPersonaStore(path string) (*PersonaStore, error) {
	if path == "" {
		path = "configs/persona.json"
	}
	abs, _ := filepath.Abs(path)
	ps := &PersonaStore{
		path:           abs,
		Personas:       make(map[string]PersonaConfig),
		DefaultPersona: "default",
	}
	if err := ps.Load(); err != nil {
		return ps, err
	}
	return ps, nil
}

// Load reads persona.json.
func (p *PersonaStore) Load() error {
	data, err := os.ReadFile(p.path)
	if err != nil {
		if os.IsNotExist(err) {
			p.mu.Lock()
			p.Personas["default"] = PersonaConfig{
				SystemPrompt: "You are a helpful assistant.",
				Temperature:  0.7,
				TopP:         1.0,
			}
			p.mu.Unlock()
			return nil
		}
		return err
	}
	var raw struct {
		Personas       map[string]PersonaConfig `json:"personas"`
		DefaultPersona string                   `json:"default_persona"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.mu.Lock()
	p.Personas = raw.Personas
	if raw.DefaultPersona != "" {
		p.DefaultPersona = raw.DefaultPersona
	}
	if len(p.Personas) == 0 {
		p.Personas["default"] = PersonaConfig{
			SystemPrompt: "You are a helpful assistant.",
			Temperature:  0.7,
			TopP:         1.0,
		}
	}
	p.mu.Unlock()
	return nil
}

// Save writes persona.json.
func (p *PersonaStore) Save() error {
	p.mu.RLock()
	data, err := json.MarshalIndent(map[string]any{
		"version":         "1.0",
		"personas":        p.Personas,
		"default_persona": p.DefaultPersona,
	}, "", "  ")
	p.mu.RUnlock()
	if err != nil {
		return err
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, p.path)
}

// Get returns persona by id.
func (p *PersonaStore) Get(id string) (PersonaConfig, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if id == "" {
		id = p.DefaultPersona
	}
	pc, ok := p.Personas[id]
	return pc, ok
}

// Set adds or updates a persona.
func (p *PersonaStore) Set(id string, pc PersonaConfig) {
	p.mu.Lock()
	p.Personas[id] = pc
	p.mu.Unlock()
}

// SetDefault sets default persona id.
func (p *PersonaStore) SetDefault(id string) {
	p.mu.Lock()
	if id != "" {
		p.DefaultPersona = id
	}
	p.mu.Unlock()
}

// Default returns the default persona id.
func (p *PersonaStore) Default() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.DefaultPersona
}

// List returns all persona ids (sorted).
func (p *PersonaStore) List() []string {
	p.mu.RLock()
	ids := make([]string, 0, len(p.Personas))
	for k := range p.Personas {
		ids = append(ids, k)
	}
	p.mu.RUnlock()
	sort.Strings(ids)
	return ids
}
