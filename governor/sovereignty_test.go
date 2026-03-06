package governor

import (
	"testing"
)

func TestSovereigntyMachine_Switch(t *testing.T) {
	m := NewSovereigntyMachine()
	m.Switch(ModeAirlock, []string{"api.example.com"})
	s := m.State()
	if s.Mode != ModeAirlock {
		t.Errorf("want airlock, got %s", s.Mode)
	}
	if len(s.Domains) != 1 || s.Domains[0] != "api.example.com" {
		t.Errorf("want [api.example.com], got %v", s.Domains)
	}
}

func TestSovereigntyMachine_TripleCheck(t *testing.T) {
	m := NewSovereigntyMachine()
	m.Switch(ModeShadow, nil)
	if m.TripleCheck("api.example.com", "443", "https") {
		t.Error("shadow should block all")
	}
	m.Switch(ModeAirlock, []string{"api.example.com"})
	if !m.TripleCheck("api.example.com", "443", "https") {
		t.Error("airlock should allow whitelisted")
	}
}
