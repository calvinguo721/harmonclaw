// Package bus provides DepthController: nested depth limits for Bus message spawning.
// Borrows maxSpawnDepth / maxChildrenPerAgent from OpenClaw v3.13.
package bus

import (
	"fmt"
	"sync"
)

const (
	// MaxSpawnDepth is the maximum nesting depth for spawned tasks.
	MaxSpawnDepth = 2
	// MaxChildrenPerCore is the maximum child tasks per core.
	MaxChildrenPerCore = 5
)

// DepthController limits spawn depth and children per core.
type DepthController struct {
	mu       sync.RWMutex
	depths   map[string]int
	children map[string]int
}

// NewDepthController creates a depth controller.
func NewDepthController() *DepthController {
	return &DepthController{
		depths:   make(map[string]int),
		children: make(map[string]int),
	}
}

// CanSpawn returns true if coreID can spawn a new child.
func (dc *DepthController) CanSpawn(coreID string) bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	d := dc.depths[coreID]
	c := dc.children[coreID]
	return d < MaxSpawnDepth && c < MaxChildrenPerCore
}

// Spawn records a child spawn from parent. Returns error if depth limit exceeded.
func (dc *DepthController) Spawn(parentID, childID string) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	parentDepth := dc.depths[parentID]
	if parentDepth >= MaxSpawnDepth {
		return fmt.Errorf("max spawn depth %d reached for %s", MaxSpawnDepth, parentID)
	}
	dc.depths[childID] = parentDepth + 1
	dc.children[parentID]++
	return nil
}

// Release decrements child count when a spawned task completes.
func (dc *DepthController) Release(parentID string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	if dc.children[parentID] > 0 {
		dc.children[parentID]--
	}
}

// ReleaseDepth removes depth for a completed child.
func (dc *DepthController) ReleaseDepth(childID string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	delete(dc.depths, childID)
}
