// Package edge provides offline mode for edge clients.
package edge

import (
	"net/http"
	"sync"
	"time"
)

const (
	offlineCacheSize   = 100
	offlineCheckPeriod = 10 * time.Second
)

// OfflineManager handles offline detection and local cache.
type OfflineManager struct {
	mu         sync.RWMutex
	backendURL string
	client     *http.Client
	offline    bool
	lastCheck  time.Time
	cache      []CachedMessage
	pending    []CachedMessage
}

// CachedMessage is a conversation turn for offline cache.
type CachedMessage struct {
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Time      time.Time `json:"time"`
}

// NewOfflineManager creates an offline manager.
func NewOfflineManager(backendURL string) *OfflineManager {
	return &OfflineManager{
		backendURL: backendURL,
		client:     &http.Client{Timeout: 5 * time.Second},
		cache:      make([]CachedMessage, 0, offlineCacheSize),
		pending:    make([]CachedMessage, 0),
	}
}

// Check updates offline status by pinging backend.
func (o *OfflineManager) Check() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	resp, err := o.client.Get(o.backendURL + "/v1/health")
	wasOffline := o.offline
	o.offline = err != nil || resp == nil || resp.StatusCode != 200
	o.lastCheck = time.Now()
	if resp != nil {
		resp.Body.Close()
	}
	if wasOffline && !o.offline {
		o.pending = make([]CachedMessage, len(o.cache))
		copy(o.pending, o.cache)
	}
	return o.offline
}

// IsOffline returns current offline state.
func (o *OfflineManager) IsOffline() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.offline
}

// Record adds a message to local cache.
func (o *OfflineManager) Record(sessionID, role, content string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cache = append(o.cache, CachedMessage{SessionID: sessionID, Role: role, Content: content, Time: time.Now()})
	if len(o.cache) > offlineCacheSize {
		o.cache = o.cache[len(o.cache)-offlineCacheSize:]
	}
}

// GetCache returns cached messages for offline display.
func (o *OfflineManager) GetCache() []CachedMessage {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]CachedMessage, len(o.cache))
	copy(out, o.cache)
	return out
}

// GetPendingSync returns messages to sync when back online.
func (o *OfflineManager) GetPendingSync() []CachedMessage {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]CachedMessage, len(o.pending))
	copy(out, o.pending)
	return out
}

// ClearPending clears pending after successful sync.
func (o *OfflineManager) ClearPending() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.pending = nil
}

