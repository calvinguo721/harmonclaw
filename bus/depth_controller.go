// Package bus provides DepthController for limiting nested spawn depth and children per core,
// inspired by OpenClaw v3.13 maxSpawnDepth / maxChildrenPerAgent.
package bus

import (
	"fmt"
	"sync"
)

const (
	// MaxSpawnDepth is the maximum nesting depth for spawned cores.
	MaxSpawnDepth = 2
	// MaxChildrenPerCore is the maximum number of children each core may spawn.
	MaxChildrenPerCore = 5
)

// DepthController limits spawn depth and children count per core.
type DepthController struct {
	mu       sync.RWMutex
	depths   map[string]int
	children map[string]int
}

// NewDepthController creates a new depth controller.
func NewDepthController() *DepthController {
	return &DepthController{
		depths:   make(map[string]int),
		children: make(map[string]int),
	}
}

// CanSpawn returns true if the core may spawn a child (depth and children limit not exceeded).
func (dc *DepthController) CanSpawn(coreID string) bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	d := dc.depths[coreID]
	c := dc.children[coreID]
	return d < MaxSpawnDepth && c < MaxChildrenPerCore
}

// Spawn records a spawn from parent to child. Returns error if limits exceeded.
func (dc *DepthController) Spawn(parentID, childID string) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	parentDepth := dc.depths[parentID]
	if parentDepth >= MaxSpawnDepth {
		return fmt.Errorf("max spawn depth %d reached for %s", MaxSpawnDepth, parentID)
	}
	if dc.children[parentID] >= MaxChildrenPerCore {
		return fmt.Errorf("max children %d reached for %s", MaxChildrenPerCore, parentID)
	}
	dc.depths[childID] = parentDepth + 1
	dc.children[parentID]++
	return nil
}

// Depth returns the spawn depth of a core (0 if unknown).
func (dc *DepthController) Depth(coreID string) int {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.depths[coreID]
}

// ChildrenCount returns the number of children spawned by a core.
func (dc *DepthController) ChildrenCount(coreID string) int {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.children[coreID]
}
