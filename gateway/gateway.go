package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"harmonclaw/architect"
	"harmonclaw/butler"
	"harmonclaw/llm"
	"harmonclaw/skills"
	"harmonclaw/viking"
)

var SovereigntyMode = "airlock"

type Server struct {
	Addr      string
	Mux       *http.ServeMux
	Butler    *butler.Agent
	Architect *architect.Agent
	Ledger    viking.Ledger
}

func New(addr string, b *butler.Agent, a *architect.Agent, ledger viking.Ledger) *Server {
	s := &Server{
		Addr:      addr,
		Mux:       http.NewServeMux(),
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

	s.Mux.Handle("/", http.FileServer(http.Dir("web")))
}

func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.Addr, sovereigntyWall(s.Mux))
}

// --- sovereignty middleware ---

func sovereigntyWall(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if SovereigntyMode == "shadow" && strings.HasPrefix(r.URL.Path, "/v1/") {
			log.Printf("SOVEREIGNTY shadow-block: %s %s", r.Method, r.URL.Path)
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
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("butler chat error: %v", err)
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
