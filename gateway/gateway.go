package gateway

import (
	"encoding/json"
	"expvar"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"harmonclaw/architect"
	"harmonclaw/butler"
	"harmonclaw/governor"
	"harmonclaw/governor/ironclaw"
	"harmonclaw/llm"
	"harmonclaw/skills"
	"harmonclaw/viking"
)

var SovereigntyMode = "airlock"

type Server struct {
	Addr      string
	Mux       *http.ServeMux
	Governor  *governor.Governor
	Butler    *butler.Butler
	Architect *architect.Architect
	Ledger    viking.Ledger
	Policies  []ironclaw.Policy
	Version   string
}

func New(addr string, gov *governor.Governor, b *butler.Butler, a *architect.Architect, ledger viking.Ledger, policies []ironclaw.Policy, version string) *Server {
	s := &Server{
		Addr:      addr,
		Mux:       http.NewServeMux(),
		Governor:  gov,
		Butler:    b,
		Architect: a,
		Ledger:    ledger,
		Policies:  policies,
		Version:   version,
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

	s.Mux.Handle("GET /static/", http.StripPrefix("/static", http.FileServer(http.Dir("web"))))
	s.Mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "" {
			http.ServeFile(w, r, "web/index.html")
			return
		}
		http.FileServer(http.Dir("web")).ServeHTTP(w, r)
	}))
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

	skillNames := make([]string, 0, len(skills.Registry))
	for id := range skills.Registry {
		skillNames = append(skillNames, id)
	}
	sort.Strings(skillNames)

	writeJSON(w, http.StatusOK, map[string]any{
		"governor":  map[string]any{"mode": SovereigntyMode, "status": govStatus},
		"butler":    map[string]string{"status": butlerStatus},
		"architect": map[string]any{"status": archStatus, "registered_skills": skillNames},
		"overall":   overall,
		"version":   s.Version,
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
		s.Ledger.Record(viking.LedgerEntry{
			OperatorID: "default",
			ActionType: "chat",
			Resource:   "chat",
			Result:     "fail",
			ClientIP:   r.RemoteAddr,
			Timestamp:  time.Now().Format(time.RFC3339),
			ActionID:   GetActionID(r.Context()),
		})
		Log(r.Context(), "butler chat error: %v", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.Ledger.Record(viking.LedgerEntry{
		OperatorID: "default",
		ActionType: "chat",
		Resource:   "chat",
		Result:     "success",
		ClientIP:   r.RemoteAddr,
		Timestamp:  time.Now().Format(time.RFC3339),
		ActionID:   GetActionID(r.Context()),
	})
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

	skillID := req.SkillID
	if skillID == "" {
		skillID = req.SkillName
	}
	if skillID == "" {
		writeError(w, http.StatusBadRequest, "skill_id or skill_name required")
		return
	}

	userID := "default"
	if !s.Governor.Quota().Allow(userID, skillID) {
		writeError(w, http.StatusTooManyRequests, "quota exceeded")
		return
	}
	defer s.Governor.Quota().Release(userID)

	token := ""
	if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
		token = strings.TrimPrefix(ah, "Bearer ")
	}
	classification := "public"
	if req.Args != nil && req.Args["classification"] != "" {
		classification = req.Args["classification"]
	}
	if policy := s.findPolicy(skillID); policy.SkillID != "" {
		if err := ironclaw.Enforce(policy, ironclaw.Request{
			UserID:         userID,
			SkillID:        skillID,
			Token:          token,
			Classification: classification,
		}); err != nil {
			s.Ledger.Record(viking.LedgerEntry{
				OperatorID: "default",
				ActionType: "skill_exec",
				Resource:   skillID,
				Result:     "fail",
				ClientIP:   r.RemoteAddr,
				Timestamp:  time.Now().Format(time.RFC3339),
				ActionID:   GetActionID(r.Context()),
			})
			writeJSON(w, http.StatusForbidden, blockResponse{
				Error:     "IRONCLAW",
				RiskLevel: "CRITICAL",
				Reason:    err.Error(),
			})
			return
		}
	}

	check := s.Architect.HandleSkill(skillID)
	if !check.Allowed {
		s.Ledger.Record(viking.LedgerEntry{
			OperatorID: "default",
			ActionType: "skill_exec",
			Resource:   skillID,
			Result:     "fail",
			ClientIP:   r.RemoteAddr,
			Timestamp:  time.Now().Format(time.RFC3339),
			ActionID:   GetActionID(r.Context()),
		})
		writeJSON(w, http.StatusForbidden, blockResponse{
			Error:     "BLOCKED",
			RiskLevel: "CRITICAL",
			Reason:    check.Verdict,
		})
		return
	}

	sk, ok := skills.Registry[skillID]
	if !ok {
		s.Ledger.Record(viking.LedgerEntry{
			OperatorID: "default",
			ActionType: "skill_exec",
			Resource:   skillID,
			Result:     "success",
			ClientIP:   r.RemoteAddr,
			Timestamp:  time.Now().Format(time.RFC3339),
			ActionID:   GetActionID(r.Context()),
		})
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

	args := req.Args
	if args == nil {
		args = make(map[string]string)
	}
	args["sovereignty"] = SovereigntyMode
	input := skills.SkillInput{
		TraceID:   GetActionID(r.Context()),
		Text:      text,
		Args:      args,
		LocalOnly: true,
	}
	if input.TraceID == "" {
		input.TraceID = fmt.Sprintf("%d", time.Now().UnixMilli())
	}
	output := sk.Execute(input)

	result := "fail"
	if output.Status == "ok" {
		result = "success"
	}
	s.Ledger.Record(viking.LedgerEntry{
		OperatorID: "default",
		ActionType: "skill_exec",
		Resource:   skillID,
		Result:     result,
		ClientIP:   r.RemoteAddr,
		Timestamp:  time.Now().Format(time.RFC3339),
		ActionID:   GetActionID(r.Context()),
	})
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
	SkillID   string            `json:"skill_id"`
	SkillName string            `json:"skill_name"`
	Input     string            `json:"input"`
	Text      string            `json:"text"`
	Args      map[string]string `json:"args"`
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

func (s *Server) findPolicy(skillID string) ironclaw.Policy {
	for _, p := range s.Policies {
		if p.SkillID == skillID {
			return p
		}
	}
	return ironclaw.Policy{}
}
