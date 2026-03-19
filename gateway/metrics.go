// Package gateway provides expvar metrics for endpoints and skills.
package gateway

import (
	"expvar"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	endpointCounts = expvar.NewMap("endpoint_requests")
	skillCounts   = expvar.NewMap("skill_calls")
	latencySum    int64
	latencyCount  int64
	latencyMu     sync.Mutex
)

func init() {
	expvar.Publish("endpoint_latency_avg_ms", expvar.Func(func() any {
		latencyMu.Lock()
		defer latencyMu.Unlock()
		if latencyCount == 0 {
			return 0
		}
		return latencySum / latencyCount
	}))
}

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		next.ServeHTTP(w, r)
		key := r.Method + " " + r.URL.Path
		endpointCounts.Add(key, 1)
		latencyMu.Lock()
		latencySum += time.Since(start).Milliseconds()
		latencyCount++
		latencyMu.Unlock()
	})
}

func recordSkillCall(skillID string) {
	skillCounts.Add(skillID, 1)
}
