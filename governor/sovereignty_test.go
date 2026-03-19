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
	m.Switch(ModeOpenSea, []string{"*"})
	if !m.TripleCheck("any.com", "443", "https") {
		t.Error("opensea with * should allow all")
	}
}

func TestGeoIPCheck(t *testing.T) {
	if !GeoIPCheck("127.0.0.1") {
		t.Error("loopback should be domestic")
	}
	if !GeoIPCheck("10.0.0.1") {
		t.Error("private should be domestic")
	}
	if GeoIPCheck("8.8.8.8") {
		t.Error("public should not be domestic by default")
	}
}

func TestAllowedBoardsCheck(t *testing.T) {
	if !AllowedBoardsCheck("riscv64") {
		t.Error("riscv64 should be in allowed-boards.json")
	}
	if !AllowedBoardsCheck("arm64") {
		t.Error("arm64 should be in allowed-boards.json")
	}
}
