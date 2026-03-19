// Package governor provides security, quota, and policy enforcement.
package governor

import (
	"log"
	"sync"
	"time"

	"harmonclaw/bus"
	"harmonclaw/viking"
)

type Governor struct {
	quota  *Quota
	ledger viking.Ledger

	mu     sync.Mutex
	status string
}

func New(ledger viking.Ledger) *Governor {
	return &Governor{
		quota:  NewQuota(),
		ledger: ledger,
		status: "ok",
	}
}

func (g *Governor) Quota() *Quota { return g.quota }

func (g *Governor) Status() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.status
}

func (g *Governor) SetOK() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.status = "ok"
}

func (g *Governor) SetDegraded() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.status = "degraded"
}

func (g *Governor) RequestGrant(core, peerID string) bool {
	if core == "governor" {
		log.Printf("governor: DENY grant %s -> %s", core, peerID)
		return false
	}
	log.Printf("governor: GRANT %s -> %s", core, peerID)
	return true
}

func (g *Governor) Pulse() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		bus.Send(bus.Message{
			From:    bus.Governor,
			Type:    "pulse",
			Payload: g.Status(),
		})
	}
}
