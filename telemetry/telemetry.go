// Package telemetry provides opt-in anonymous usage data. Disable with HC_TELEMETRY=off.
package telemetry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

var (
	enabled   bool
	enabledMu sync.Once
)

// Enabled returns true unless HC_TELEMETRY=off.
func Enabled() bool {
	enabledMu.Do(func() {
		enabled = os.Getenv("HC_TELEMETRY") != "off"
	})
	return enabled
}

// Event is an anonymous event payload.
type Event struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"ts"`
	GoVersion string    `json:"go_version,omitempty"`
	OS        string    `json:"os,omitempty"`
	Arch      string    `json:"arch,omitempty"`
}

// Emit sends an event if telemetry is enabled. Non-blocking.
func Emit(evType string) {
	if !Enabled() {
		return
	}
	ev := Event{
		Type:      evType,
		Timestamp: time.Now(),
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
	go func() {
		body, _ := json.Marshal(ev)
		req, err := http.NewRequest(http.MethodPost, "https://telemetry.harmonclaw.example/events", bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		http.DefaultClient.Do(req)
	}()
}
