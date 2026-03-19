// Package governor (sovereignty) provides three-mode state machine and triple-check for outbound requests.
package governor

import (
	"strconv"
	"strings"
	"sync"

	"harmonclaw/bus"
)

const (
	ModeShadow  = "shadow"
	ModeAirlock = "airlock"
	ModeOpenSea = "opensea"
)

// SovereigntyState holds mode and whitelist configuration.
type SovereigntyState struct {
	Mode    string
	Domains []string
	Ports   []int // allowed ports, empty = all
	Schemes []string // http, https
}

// SovereigntyMachine manages three-mode state and broadcasts bus events on switch.
type SovereigntyMachine struct {
	mu    sync.RWMutex
	state SovereigntyState
}

// NewSovereigntyMachine creates a new state machine.
func NewSovereigntyMachine() *SovereigntyMachine {
	return &SovereigntyMachine{
		state: SovereigntyState{
			Mode:    ModeAirlock,
			Domains: []string{},
			Ports:   []int{80, 443},
			Schemes: []string{"http", "https"},
		},
	}
}

// Switch changes mode and domains, broadcasts bus event.
func (m *SovereigntyMachine) Switch(mode string, domains []string) {
	valid := map[string]bool{ModeShadow: true, ModeAirlock: true, ModeOpenSea: true}
	if !valid[mode] {
		mode = ModeAirlock
	}
	if domains == nil {
		domains = []string{}
	}

	m.mu.Lock()
	oldMode := m.state.Mode
	m.state.Mode = mode
	m.state.Domains = domains
	m.mu.Unlock()

	if oldMode != mode {
		bus.Send(bus.Message{
			From:    bus.Governor,
			Type:    "sovereignty_switch",
			Payload: map[string]any{"from": oldMode, "to": mode, "domains": domains},
		})
	}
}

// State returns current state (copy).
func (m *SovereigntyMachine) State() SovereigntyState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.state
	s.Domains = make([]string, len(m.state.Domains))
	copy(s.Domains, m.state.Domains)
	s.Ports = make([]int, len(m.state.Ports))
	copy(s.Ports, m.state.Ports)
	s.Schemes = make([]string, len(m.state.Schemes))
	copy(s.Schemes, m.state.Schemes)
	return s
}

// TripleCheck validates domain, port, and scheme. Returns true if allowed.
func (m *SovereigntyMachine) TripleCheck(host, portStr, scheme string) bool {
	m.mu.RLock()
	s := m.state
	domains := make([]string, len(s.Domains))
	copy(domains, s.Domains)
	ports := make([]int, len(s.Ports))
	copy(ports, s.Ports)
	schemes := make([]string, len(s.Schemes))
	copy(schemes, s.Schemes)
	m.mu.RUnlock()

	if s.Mode == ModeShadow {
		return false
	}
	if s.Mode == ModeOpenSea {
		for _, d := range domains {
			if d == "*" {
				return true
			}
		}
	}

	// Domain check
	hostOnly := host
	if idx := strings.Index(host, ":"); idx >= 0 {
		hostOnly = host[:idx]
	}
	if !domainAllowed(hostOnly, s.Mode, domains) {
		return false
	}

	// Port check
	if len(ports) > 0 {
		port := 0
		if portStr != "" {
			port, _ = strconv.Atoi(portStr)
		}
		if scheme == "https" && port == 0 {
			port = 443
		}
		if scheme == "http" && port == 0 {
			port = 80
		}
		allowed := false
		for _, p := range ports {
			if p == port {
				allowed = true
				break
			}
		}
		if !allowed && port != 0 {
			return false
		}
	}

	// Scheme check
	if len(schemes) > 0 {
		scheme = strings.ToLower(scheme)
		allowed := false
		for _, sc := range schemes {
			if sc == scheme {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	return true
}

func domainAllowed(host, mode string, domains []string) bool {
	if mode == ModeOpenSea {
		for _, d := range domains {
			if d == "*" {
				return true
			}
		}
	}
	for _, d := range domains {
		if d == "*" {
			return true
		}
		if d == host {
			return true
		}
		if strings.HasPrefix(d, "*.") {
			suffix := d[1:]
			if host == suffix || strings.HasSuffix(host, suffix) {
				return true
			}
		}
	}
	return false
}

// SetPorts updates allowed ports.
func (m *SovereigntyMachine) SetPorts(ports []int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Ports = ports
}

// SetSchemes updates allowed schemes.
func (m *SovereigntyMachine) SetSchemes(schemes []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Schemes = schemes
}

