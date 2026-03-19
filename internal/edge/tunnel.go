// Package edge provides edge device registration and command channel.
package edge

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Device holds edge device info.
type Device struct {
	ID           string            `json:"id"`
	Arch         string            `json:"arch"`
	OS           string            `json:"os"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Status       string            `json:"status"`
	Metrics      map[string]any    `json:"metrics,omitempty"`
	LastSeen     time.Time         `json:"last_seen"`
	RegisteredAt time.Time         `json:"registered_at"`
}

// Tunnel manages edge device registration and heartbeat.
type Tunnel struct {
	mu      sync.RWMutex
	devices map[string]*Device
}

// NewTunnel creates a tunnel.
func NewTunnel() *Tunnel {
	return &Tunnel{devices: make(map[string]*Device)}
}

// Register adds or updates a device.
func (t *Tunnel) Register(deviceID, arch, os string, capabilities []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if d, ok := t.devices[deviceID]; ok {
		d.Arch = arch
		d.OS = os
		d.Capabilities = capabilities
		d.LastSeen = now
		d.Status = "online"
		return
	}
	t.devices[deviceID] = &Device{
		ID:           deviceID,
		Arch:         arch,
		OS:           os,
		Capabilities: capabilities,
		Status:       "online",
		LastSeen:     now,
		RegisteredAt: now,
	}
}

// Heartbeat updates device status and metrics.
func (t *Tunnel) Heartbeat(deviceID, status string, metrics map[string]any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if d, ok := t.devices[deviceID]; ok {
		d.LastSeen = time.Now()
		d.Status = status
		d.Metrics = metrics
	}
}

// List returns all devices.
func (t *Tunnel) List() []Device {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Device, 0, len(t.devices))
	for _, d := range t.devices {
		dc := *d
		if time.Since(dc.LastSeen) > 2*time.Minute {
			dc.Status = "offline"
		}
		out = append(out, dc)
	}
	return out
}

// Command is a placeholder for command dispatch (sovereignty switch etc).
func (t *Tunnel) Command(deviceID, command string, payload map[string]any) error {
	t.mu.RLock()
	_, ok := t.devices[deviceID]
	t.mu.RUnlock()
	if !ok {
		return nil
	}
	return nil
}

// HandleRegister handles POST /v1/edge/register.
func HandleRegister(t *Tunnel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		var req struct {
			DeviceID     string   `json:"device_id"`
			Arch         string   `json:"arch"`
			OS           string   `json:"os"`
			Capabilities []string `json:"capabilities"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil || req.DeviceID == "" {
			http.Error(w, "invalid body", 400)
			return
		}
		t.Register(req.DeviceID, req.Arch, req.OS, req.Capabilities)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "device_id": req.DeviceID})
	}
}

// HandleHeartbeat handles POST /v1/edge/heartbeat.
func HandleHeartbeat(t *Tunnel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		var req struct {
			DeviceID string         `json:"device_id"`
			Status   string         `json:"status"`
			Metrics  map[string]any `json:"metrics"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil || req.DeviceID == "" {
			http.Error(w, "invalid body", 400)
			return
		}
		t.Heartbeat(req.DeviceID, req.Status, req.Metrics)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// HandleDevices handles GET /v1/edge/devices.
func HandleDevices(t *Tunnel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(t.List())
	}
}

// HandleCommand handles POST /v1/edge/command.
func HandleCommand(t *Tunnel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		var req struct {
			DeviceID string         `json:"device_id"`
			Command  string         `json:"command"`
			Payload  map[string]any `json:"payload"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil || req.DeviceID == "" || req.Command == "" {
			http.Error(w, "invalid body", 400)
			return
		}
		t.Command(req.DeviceID, req.Command, req.Payload)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
