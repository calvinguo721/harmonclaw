// Package gateway provides middleware chain: recover→firewall→ratelimit→auth→action_id→ironclaw→handler→ledger.
package gateway

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"harmonclaw/governor"
	"harmonclaw/viking"
)

// Chain builds: recover→firewall→ratelimit→auth→action_id→ironclaw→metrics→handler→ledger
func Chain(mux http.Handler, ledger viking.Ledger, firewall *governor.Firewall, rateLimiter *governor.TripleRateLimiter, authEnabled bool) http.Handler {
	h := mux
	h = ledgerMiddleware(ledger, h)
	h = metricsMiddleware(h)
	h = ironclawMiddleware(h)
	h = actionMiddleware(h)
	if authEnabled {
		h = authMiddleware(h)
	}
	if rateLimiter != nil {
		h = rateLimitMiddleware(rateLimiter, h)
	}
	if firewall != nil {
		h = firewall.Wrap(h)
	}
	h = recoverMiddleware(ledger, h)
	return h
}

func rateLimitMiddleware(rl *governor.TripleRateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		userID := "default"
		skillID := r.URL.Path
		if ok, retryAfter := rl.Allow(userID, skillID); !ok {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func ironclawMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if SovereigntyMode == "shadow" && strings.HasPrefix(r.URL.Path, "/v1/") {
			Log(r.Context(), "SOVEREIGNTY shadow-block: %s %s", r.Method, r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"SOVEREIGNTY: shadow mode"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func ledgerMiddleware(ledger viking.Ledger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if ledger != nil && strings.HasPrefix(r.URL.Path, "/v1/") {
			actionID := GetActionID(r.Context())
			ledger.Record(viking.LedgerEntry{
				OperatorID: "gateway",
				ActionType: "request",
				Resource:   r.URL.Path,
				Result:     "success",
				ClientIP:   r.RemoteAddr,
				Timestamp:  time.Now().Format(time.RFC3339),
				ActionID:   actionID,
			})
		}
	})
}
