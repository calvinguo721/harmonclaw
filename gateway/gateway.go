// Package gateway provides HTTP routing and handlers for HarmonClaw.
package gateway

import (
	"context"
	"encoding/json"
	"expvar"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"harmonclaw/architect"
	"harmonclaw/butler"
	"harmonclaw/governor"
	"harmonclaw/governor/ironclaw"
	"harmonclaw/viking"
)

func authEnabled() bool {
	return os.Getenv("HC_AUTH_ENABLED") == "true"
}

var SovereigntyMode = "airlock"

type Server struct {
	Addr          string
	Mux           *http.ServeMux
	httpServer    *http.Server
	Governor      *governor.Governor
	Butler        *butler.Butler
	Architect     *architect.Architect
	Ledger        viking.Ledger
	Policies      []ironclaw.Policy
	Version       string
	EngramBaseDir string
	Audit         *governor.AuditEngine
	VikingStore   *viking.KVStore
	VikingSearch  *viking.SearchIndex
	VikingSnap    *viking.SnapshotManager
	Firewall      *governor.Firewall
	RateLimiter   *governor.TripleRateLimiter
}

func New(addr string, gov *governor.Governor, b *butler.Butler, a *architect.Architect, ledger viking.Ledger, policies []ironclaw.Policy, version string) *Server {
	return NewWithEngramDir(addr, gov, b, a, ledger, policies, version, "")
}

func NewWithEngramDir(addr string, gov *governor.Governor, b *butler.Butler, a *architect.Architect, ledger viking.Ledger, policies []ironclaw.Policy, version string, engramBaseDir string) *Server {
	s := &Server{
		Addr:          addr,
		Mux:           http.NewServeMux(),
		Governor:      gov,
		Butler:        b,
		Architect:     a,
		Ledger:        ledger,
		Policies:      policies,
		Version:       version,
		EngramBaseDir: engramBaseDir,
	}
	if ql, ok := ledger.(viking.QueryableLedger); ok {
		s.Audit = governor.NewAuditEngine(ql)
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.Mux.HandleFunc("GET /v1/health", s.handleHealth)
	s.Mux.HandleFunc("GET /v1/governor/sovereignty", s.handleSovereigntyGet)
	s.Mux.HandleFunc("POST /v1/governor/sovereignty", s.handleSovereigntyPost)
	s.Mux.HandleFunc("GET /v1/governor/ratelimit", s.handleRateLimitGet)
	s.Mux.HandleFunc("PUT /v1/governor/ratelimit", s.handleRateLimitPut)
	s.Mux.HandleFunc("POST /v1/chat/completions", s.handleChat)
	s.Mux.HandleFunc("POST /v1/skills/execute", s.handleSkills)
	s.Mux.HandleFunc("POST /v1/engram/inject", s.handleEngram)
	s.Mux.HandleFunc("GET /v1/ledger/latest", s.handleLedger)
	s.Mux.HandleFunc("GET /v1/ledger/trace", s.handleLedgerTrace)
	s.Mux.HandleFunc("POST /v1/token", s.handleToken)
	s.Mux.HandleFunc("POST /v1/auth/login", s.handleAuthLogin)
	s.Mux.HandleFunc("GET /v1/version", s.handleVersion)
	s.Mux.HandleFunc("GET /v1/test/illegal", s.handleTestIllegal)
	s.Mux.HandleFunc("GET /v1/test/panic", s.handleTestPanic)
	s.Mux.HandleFunc("GET /v1/audit/query", s.handleAuditQuery)
	s.Mux.HandleFunc("POST /v1/audit/query", s.handleAuditQuery)
	s.Mux.HandleFunc("GET /v1/butler/persona", s.handlePersonaGet)
	s.Mux.HandleFunc("POST /v1/butler/persona", s.handlePersonaPost)
	s.Mux.HandleFunc("GET /v1/architect/skills", s.handleArchitectSkills)
	s.Mux.HandleFunc("POST /v1/architect/pipeline/execute", s.handlePipelineExecute)
	s.Mux.HandleFunc("GET /v1/architect/crons", s.handleArchitectCrons)
	s.Mux.HandleFunc("GET /v1/viking/snapshots", s.handleVikingSnapshots)
	s.Mux.HandleFunc("GET /v1/viking/search", s.handleVikingSearch)
	s.Mux.HandleFunc("POST /v1/viking/search", s.handleVikingSearch)
	s.Mux.Handle("GET /debug/vars", expvar.Handler())

	s.Mux.Handle("GET /static/", http.StripPrefix("/static", http.FileServer(http.Dir("web"))))
	s.Mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "" {
			http.ServeFile(w, r, "web/index.html")
			return
		}
		http.FileServer(http.Dir("web")).ServeHTTP(w, r)
	}))
}

func (s *Server) SetFirewall(f *governor.Firewall) { s.Firewall = f }
func (s *Server) SetRateLimiter(r *governor.TripleRateLimiter) { s.RateLimiter = r }

func (s *Server) ListenAndServe() error {
	h := Chain(s.Mux, s.Ledger, s.Firewall, s.RateLimiter, authEnabled())
	h = CORS(h)
	s.httpServer = &http.Server{Addr: s.Addr, Handler: h}
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server with the given timeout.
func (s *Server) Shutdown(timeout time.Duration) error {
	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

func recoverMiddleware(ledger viking.Ledger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				actionID := GetActionID(r.Context())
				ledger.Record(viking.LedgerEntry{
					OperatorID: "gateway",
					ActionType: "panic_recovered",
					Resource:   r.URL.Path,
					Result:     "fail",
					ClientIP:   r.RemoteAddr,
					Timestamp:  time.Now().Format(time.RFC3339),
					ActionID:   actionID,
				})
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/v1/token" && r.Method == "POST" {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/v1/health" || r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/v1/auth/login" && r.Method == "POST" {
			next.ServeHTTP(w, r)
			return
		}
		ah := r.Header.Get("Authorization")
		if !strings.HasPrefix(ah, "Bearer ") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid token"})
			return
		}
		token := strings.TrimPrefix(ah, "Bearer ")
		if _, err := governor.ValidateToken(token); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid token: " + err.Error()})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- sovereignty middleware ---

func sovereigntyWall(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if SovereigntyMode == "shadow" && strings.HasPrefix(r.URL.Path, "/v1/") {
			Log(r.Context(), "SOVEREIGNTY shadow-block: %s %s", r.Method, r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "SOVEREIGNTY: shadow mode — all outbound API calls physically severed",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	if r != nil && strings.Contains(r.Header.Get("Accept"), "text/html") {
		serveErrorPage(w, status, msg)
		return
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func serveErrorPage(w http.ResponseWriter, status int, msg string) {
	data, err := os.ReadFile("web/error.html")
	if err != nil {
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}
	html := strings.ReplaceAll(string(data), "{{CODE}}", strconv.Itoa(status))
	html = strings.ReplaceAll(html, "{{MSG}}", strings.ReplaceAll(msg, "<", "&lt;"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(html))
}

var (
	buildTime = "unknown"
	gitCommit = "unknown"
)

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"version":     s.Version,
		"build_time":  buildTime,
		"go_version":  runtime.Version(),
		"git_commit": gitCommit,
	})
}

func (s *Server) findPolicy(skillID string) ironclaw.Policy {
	for _, p := range s.Policies {
		if p.SkillID == skillID {
			return p
		}
	}
	return ironclaw.Policy{}
}
