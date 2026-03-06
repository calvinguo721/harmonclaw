package gateway

import (
	"encoding/json"
	"expvar"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"harmonclaw/architect"
	"harmonclaw/butler"
	"harmonclaw/governor"
	"harmonclaw/llm"
	"harmonclaw/skills"
	"harmonclaw/viking"
)

var SovereigntyMode = "airlock"

type Server struct {
	Addr      string
	Mux       *http.ServeMux
	Governor  *governor.Agent
	Butler    *butler.Agent
	Architect *architect.Agent
	Ledger    viking.Ledger
}

func New(addr string, gov *governor.Agent, b *butler.Agent, a *architect.Agent, ledger viking.Ledger) *Server {
	s := &Server{
		Addr:      addr,
		Mux:       http.NewServeMux(),
		Governor:  gov,
		Butler:    b,
		Architect: a,
		Ledger:    ledger,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.Mux.HandleFunc("GET /v1/health", s.handleHealth)
	s.Mux.HandleFunc("POST /v1/chat/completions", s.handleChat)
	s.Mux.HandleFunc("POST /v1/skills/execute", s.handleSkills)
	s.Mux.HandleFunc("POST /v1/engram/inject", s.handleEngram)
	s.Mux.HandleFunc("GET /v1/ledger/latest", s.handleLedger)
	s.Mux.HandleFunc("GET /v1/test/illegal", s.handleTestIllegal)
	s.Mux.Handle("GET /debug/vars", expvar.Handler())

	s.Mux.Handle("/", http.FileServer(http.Dir("web")))
}

func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.Addr, actionMiddleware(sovereigntyWall(s.Mux)))
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

// --- handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	govStatus := s.Governor.Status()
	butlerStatus := s.Butler.Status()
	archStatus := s.Architect.Status()

	overall := "healthy"
	if govStatus != "ok" || butlerStatus != "ok" || archStatus != "ok" {
		overall = "degraded"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"governor":  map[string]string{"mode": SovereigntyMode, "status": govStatus},
		"butler":    map[string]string{"status": butlerStatus},
		"architect": map[string]string{"status": archStatus},
		"overall":   overall,
	})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	userID := "default"
	if !s.Governor.Quota().Allow(userID, "chat") {
		writeError(w, http.StatusTooManyRequests, "quota exceeded")
		return
	}
	defer s.Governor.Quota().Release(userID)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	var req llm.Request
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	resp, err := s.Butler.HandleChat(req)
	if err != nil {
		Log(r.Context(), "butler chat error: %v", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, chatResponse{
		Choices: []chatChoice{
			{Message: llm.Message{Role: "assistant", Content: resp.Content}},
		},
	})
}

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	var req skillRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	userID := "default"
	if !s.Governor.Quota().Allow(userID, req.SkillID) {
		writeError(w, http.StatusTooManyRequests, "quota exceeded")
		return
	}
	defer s.Governor.Quota().Release(userID)

	check := s.Architect.HandleSkill(req.SkillID)
	if !check.Allowed {
		writeJSON(w, http.StatusForbidden, blockResponse{
			Error:     "BLOCKED",
			RiskLevel: "CRITICAL",
			Reason:    check.Verdict,
		})
		return
	}

	sk, ok := skills.Registry[req.SkillID]
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": check.Status,
			"result": check.Result,
		})
		return
	}

	text := req.Text
	if text == "" {
		text = req.Input
	}
	if text == "" {
		writeError(w, http.StatusBadRequest, "input text is empty")
		return
	}

	input := skills.SkillInput{
		TraceID:   fmt.Sprintf("%d", time.Now().UnixMilli()),
		Text:      text,
		Args:      req.Args,
		LocalOnly: true,
	}
	output := sk.Execute(input)
	writeJSON(w, http.StatusOK, output)
}

func (s *Server) handleEngram(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "Engram Bus: awaiting DeepSeek V4 activation", http.StatusNotImplemented)
}

func (s *Server) handleLedger(w http.ResponseWriter, _ *http.Request) {
	entries, err := s.Ledger.Latest(10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read ledger")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleTestIllegal(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("X-HarmonClaw-Alert", "True")
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error":      "ILLEGAL_ACCESS",
		"risk_level": "CRITICAL",
		"message":    "stress test triggered — this incident has been logged",
	})
}

// --- request/response types ---

type skillRequest struct {
	SkillID string            `json:"skill_id"`
	Input   string            `json:"input"`
	Text    string            `json:"text"`
	Args    map[string]string `json:"args"`
}

type blockResponse struct {
	Error     string `json:"error"`
	RiskLevel string `json:"risk_level"`
	Reason    string `json:"reason"`
}

type chatChoice struct {
	Message llm.Message `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
