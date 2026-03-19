package edge

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestTunnel_RegisterHeartbeatList(t *testing.T) {
	tun := NewTunnel()
	tun.Register("dev1", "riscv64", "linux", []string{"chat", "tts"})
	tun.Heartbeat("dev1", "ok", map[string]any{"cpu": 10})
	list := tun.List()
	if len(list) != 1 {
		t.Fatalf("want 1 device, got %d", len(list))
	}
	if list[0].ID != "dev1" || list[0].Arch != "riscv64" {
		t.Errorf("device=%+v", list[0])
	}
}

func TestTunnel_HandleRegister(t *testing.T) {
	tun := NewTunnel()
	h := HandleRegister(tun)
	body := []byte(`{"device_id":"d1","arch":"arm64","os":"linux","capabilities":["chat"]}`)
	req := httptest.NewRequest("POST", "/v1/edge/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != 200 {
		t.Errorf("register: want 200, got %d", w.Code)
	}
	list := tun.List()
	if len(list) != 1 || list[0].ID != "d1" {
		t.Errorf("list=%v", list)
	}
}

func TestTunnel_HandleHeartbeat(t *testing.T) {
	tun := NewTunnel()
	tun.Register("h1", "x", "y", nil)
	h := HandleHeartbeat(tun)
	body := []byte(`{"device_id":"h1","status":"ok","metrics":{"mem":50}}`)
	req := httptest.NewRequest("POST", "/v1/edge/heartbeat", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != 200 {
		t.Errorf("heartbeat: want 200, got %d", w.Code)
	}
}

func TestTunnel_HandleDevices(t *testing.T) {
	tun := NewTunnel()
	tun.Register("g1", "a", "b", nil)
	h := HandleDevices(tun)
	req := httptest.NewRequest("GET", "/v1/edge/devices", nil)
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != 200 {
		t.Errorf("devices: want 200, got %d", w.Code)
	}
	var list []Device
	if json.NewDecoder(w.Body).Decode(&list) != nil || len(list) != 1 {
		t.Errorf("devices response invalid")
	}
}

func TestTunnel_HandleCommand(t *testing.T) {
	tun := NewTunnel()
	tun.Register("c1", "a", "b", nil)
	h := HandleCommand(tun)
	body := []byte(`{"device_id":"c1","command":"set_mode","payload":{"mode":"shadow"}}`)
	req := httptest.NewRequest("POST", "/v1/edge/command", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != 200 {
		t.Errorf("command: want 200, got %d", w.Code)
	}
}
