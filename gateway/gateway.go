// Package gateway provides HTTP routing and handlers for HarmonClaw.
package gateway

import (
	"encoding/json"
	"expvar"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
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

func authEnabled() bool {
	return os.Getenv("HC_AUTH_ENABLED") == "true"
}

type skillGuard struct {
	*architect.Architect
}

func (g skillGuard) CheckSkill(id string) (bool, string) {
	return g.Architect.CheckSkill(id)
}

var SovereigntyMode = "airlock"

type Server struct {
	Addr          string
	Mux           *http.ServeMux
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
	s.Mux.HandleFunc("POST /v1/chat/completions", s.handleChat)
	s.Mux.HandleFunc("POST /v1/skills/execute", s.handleSkills)
	s.Mux.HandleFunc("POST /v1/engram/inject", s.handleEngram)
	s.Mux.HandleFunc("GET /v1/ledger/latest", s.handleLedger)
	s.Mux.HandleFunc("GET /v1/ledger/trace", s.handleLedgerTrace)
	s.Mux.HandleFunc("POST /v1/token", s.handleToken)
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
	return http.ListenAndServe(s.Addr, h)
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

// --- handlers ---

func (s *Server) handleSovereigntyGet(w http.ResponseWriter, _ *http.Request) {
	mode, domains := governor.GetSovereigntyMode()
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":    mode,
		"domains": domains,
	})
}

func (s *Server) handleSovereigntyPost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()
	var req struct {
		Mode    string   `json:"mode"`
		Domains []string `json:"domains"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Mode == "" {
		req.Mode = "airlock"
	}
	validModes := map[string]bool{"shadow": true, "airlock": true, "opensea": true}
	if !validModes[req.Mode] {
		writeError(w, http.StatusBadRequest, "invalid mode: "+req.Mode)
		return
	}
	domains := req.Domains
	if domains == nil {
		domains = []string{}
	}
	governor.SetSovereigntyMode(req.Mode, domains)
	SovereigntyMode = req.Mode
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":    req.Mode,
		"domains": domains,
	})
}

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
	actionID := GetActionID(r.Context())
	userID := "default"
	if !s.Governor.Quota().Allow(userID, "chat") {
		writeError(w, http.StatusTooManyRequests, "quota exceeded")
		return
	}
	defer s.Governor.Quota().Release(userID)

	chatPolicy := s.findPolicy("chat")
	if chatPolicy.SkillID != "" {
		token := ""
		if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
			token = strings.TrimPrefix(ah, "Bearer ")
		}
		if err := ironclaw.Enforce(chatPolicy, ironclaw.Request{
			UserID:         userID,
			SkillID:        "chat",
			Token:          token,
			Classification: "public",
		}); err != nil {
			s.Ledger.Record(viking.LedgerEntry{
				OperatorID: "default",
				ActionType: SovereigntyMode + ":chat",
				Resource:   "chat",
				Result:     "fail",
				ClientIP:   r.RemoteAddr,
				Timestamp:  time.Now().Format(time.RFC3339),
				ActionID:   actionID,
			})
			writeJSON(w, http.StatusForbidden, blockResponse{
				Error:     "IRONCLAW",
				RiskLevel: "CRITICAL",
				Reason:    err.Error(),
			})
			return
		}
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	var chatReq llm.Request
	if err := json.Unmarshal(body, &chatReq); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if chatReq.Stream {
		s.handleChatStream(w, r, actionID, chatReq)
		return
	}

	resp, err := s.Butler.HandleChat(chatReq)
	if err != nil {
		s.Ledger.Record(viking.LedgerEntry{
			OperatorID: "default",
			ActionType: SovereigntyMode + ":llm_call",
			Resource:   "chat",
			Result:     "fail",
			ClientIP:   r.RemoteAddr,
			Timestamp:  time.Now().Format(time.RFC3339),
			ActionID:   actionID,
		})
		Log(r.Context(), "butler chat error: %v", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.Ledger.Record(viking.LedgerEntry{
		OperatorID: "default",
		ActionType: SovereigntyMode + ":llm_call",
		Resource:   "chat",
		Result:     "success",
		ClientIP:   r.RemoteAddr,
		Timestamp:  time.Now().Format(time.RFC3339),
		ActionID:   actionID,
	})
	writeJSON(w, http.StatusOK, chatResponse{
		ActionID: actionID,
		Choices: []chatChoice{
			{Message: llm.Message{Role: "assistant", Content: resp.Content}},
		},
	})
}

func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request, actionID string, chatReq llm.Request) {
	ch, sessionID, err := s.Butler.HandleChatStream(chatReq)
	if err != nil {
		s.Ledger.Record(viking.LedgerEntry{
			OperatorID: "default",
			ActionType: SovereigntyMode + ":llm_call",
			Resource:   "chat",
			Result:     "fail",
			ClientIP:   r.RemoteAddr,
			Timestamp:  time.Now().Format(time.RFC3339),
			ActionID:   actionID,
		})
		Log(r.Context(), "butler chat stream error: %v", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	var fullContent strings.Builder
	for chunk := range ch {
		fullContent.WriteString(chunk)
		evt := map[string]any{
			"action_id": actionID,
			"choices":   []any{map[string]any{"delta": map[string]any{"content": chunk}}},
		}
		evtBytes, _ := json.Marshal(evt)
		io.WriteString(w, "data: ")
		w.Write(evtBytes)
		io.WriteString(w, "\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	s.Butler.SaveStreamedResponse("default", sessionID, fullContent.String())
	s.Ledger.Record(viking.LedgerEntry{
		OperatorID: "default",
		ActionType: SovereigntyMode + ":llm_call",
		Resource:   "chat",
		Result:     "success",
		ClientIP:   r.RemoteAddr,
		Timestamp:  time.Now().Format(time.RFC3339),
		ActionID:   actionID,
	})
	io.WriteString(w, "data: [DONE]\n\n")
}

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	actionID := GetActionID(r.Context())
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
				ActionType: SovereigntyMode + ":skill_exec",
				Resource:   skillID,
				Result:     "fail",
				ClientIP:   r.RemoteAddr,
				Timestamp:  time.Now().Format(time.RFC3339),
				ActionID:   actionID,
			})
			writeJSON(w, http.StatusForbidden, blockResponse{
				Error:     "IRONCLAW",
				RiskLevel: "CRITICAL",
				Reason:    err.Error(),
			})
			return
		}
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
		TraceID:   actionID,
		Text:      text,
		Args:      args,
		LocalOnly: true,
	}
	if input.TraceID == "" {
		input.TraceID = fmt.Sprintf("%d", time.Now().UnixMilli())
	}

	output, err := s.Architect.ExecuteSkill(skillID, input)
	if err != nil {
		if err == architect.ErrBackpressure {
			writeError(w, http.StatusServiceUnavailable, "skill execution backlogged")
			return
		}
		s.Ledger.Record(viking.LedgerEntry{
			OperatorID: "default",
			ActionType: SovereigntyMode + ":skill_exec",
			Resource:   skillID,
			Result:     "fail",
			ClientIP:   r.RemoteAddr,
			Timestamp:  time.Now().Format(time.RFC3339),
			ActionID:   actionID,
		})
		writeJSON(w, http.StatusInternalServerError, output)
		return
	}

	result := "fail"
	if output.Status == "ok" {
		result = "success"
	}
	s.Ledger.Record(viking.LedgerEntry{
		OperatorID: "default",
		ActionType: SovereigntyMode + ":skill_exec",
		Resource:   skillID,
		Result:     result,
		ClientIP:   r.RemoteAddr,
		Timestamp:  time.Now().Format(time.RFC3339),
		ActionID:   actionID,
	})
	writeJSON(w, http.StatusOK, output)
}

func (s *Server) handleEngram(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	var req engramRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if req.Source != "user" && req.Source != "system" {
		req.Source = "user"
	}
	if req.Classification == "" {
		req.Classification = "public"
	}
	validClass := map[string]bool{"public": true, "internal": true, "sensitive": true, "secret": true}
	if !validClass[req.Classification] {
		req.Classification = "public"
	}

	actionID := GetActionID(r.Context())
	if actionID == "" {
		actionID = fmt.Sprintf("%d", time.Now().UnixMilli())
	}
	ts := time.Now().Format("20060102150405")
	filename := ts + "_" + actionID + ".txt"
	path, err := viking.EngramPathWithBase(s.EngramBaseDir, filename)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "engram path: "+err.Error())
		return
	}

	content := fmt.Sprintf("# source=%s\n# action_id=%s\n\n%s", req.Source, actionID, req.Text)
	if _, err := viking.SafeWrite(path, []byte(content), req.Classification); err != nil {
		writeError(w, http.StatusInternalServerError, "engram write: "+err.Error())
		return
	}

	s.Ledger.Record(viking.LedgerEntry{
		OperatorID: "default",
		ActionType: "engram_inject",
		Resource:   path,
		Result:     "success",
		ClientIP:   r.RemoteAddr,
		Timestamp:  time.Now().Format(time.RFC3339),
		ActionID:   actionID,
	})

	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"action_id": actionID,
		"path":      path,
	})
}

func (s *Server) handleLedger(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if n := r.URL.Query().Get("limit"); n != "" {
		if v, err := strconv.Atoi(n); err == nil && v > 0 && v <= 200 {
			limit = v
		}
	}
	entries, err := s.Ledger.Latest(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read ledger")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleLedgerTrace(w http.ResponseWriter, r *http.Request) {
	actionID := r.URL.Query().Get("action_id")
	if actionID == "" {
		writeError(w, http.StatusBadRequest, "action_id query parameter required")
		return
	}
	entries, err := s.Ledger.TraceByActionID(actionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to trace ledger")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"action_id": actionID,
		"chain":     entries,
	})
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	userID := "default"
	if body, err := io.ReadAll(r.Body); err == nil && len(body) > 0 {
		var req struct {
			UserID string `json:"user_id"`
		}
		if json.Unmarshal(body, &req) == nil && req.UserID != "" {
			userID = req.UserID
		}
	}
	token, err := governor.GenerateToken(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token, "user_id": userID})
}

func (s *Server) handleTestIllegal(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("X-HarmonClaw-Alert", "True")
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error":      "ILLEGAL_ACCESS",
		"risk_level": "CRITICAL",
		"message":    "stress test triggered — this incident has been logged",
	})
}

func (s *Server) handleTestPanic(w http.ResponseWriter, _ *http.Request) {
	panic("smoke test panic")
}

func (s *Server) handleAuditQuery(w http.ResponseWriter, r *http.Request) {
	if s.Audit == nil {
		writeError(w, http.StatusNotImplemented, "audit not available")
		return
	}
	var req struct {
		TimeFrom   string `json:"time_from"`
		TimeTo     string `json:"time_to"`
		OperatorID string `json:"operator_id"`
		ActionType string `json:"action_type"`
		Resource   string `json:"resource"`
		Offset     int    `json:"offset"`
		Limit      int    `json:"limit"`
	}
	if r.Method == http.MethodPost {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		json.Unmarshal(body, &req)
	} else {
		req.TimeFrom = r.URL.Query().Get("time_from")
		req.TimeTo = r.URL.Query().Get("time_to")
		req.OperatorID = r.URL.Query().Get("operator_id")
		req.ActionType = r.URL.Query().Get("action_type")
		req.Resource = r.URL.Query().Get("resource")
		if n, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil {
			req.Offset = n
		}
		if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
			req.Limit = n
		}
	}
	f := governor.QueryFilter{
		OperatorID: req.OperatorID,
		ActionType: req.ActionType,
		Resource:   req.Resource,
		Offset:     req.Offset,
		Limit:      req.Limit,
	}
	if req.TimeFrom != "" {
		t, _ := time.Parse(time.RFC3339, req.TimeFrom)
		f.TimeFrom = t
	}
	if req.TimeTo != "" {
		t, _ := time.Parse(time.RFC3339, req.TimeTo)
		f.TimeTo = t
	}
	entries, err := s.Audit.Query(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handlePersonaGet(w http.ResponseWriter, _ *http.Request) {
	if s.Butler == nil || s.Butler.Persona() == nil {
		writeJSON(w, http.StatusOK, map[string]any{"personas": []string{}, "default": "default"})
		return
	}
	ps := s.Butler.Persona()
	writeJSON(w, http.StatusOK, map[string]any{
		"personas": ps.List(),
		"default": ps.Default(),
	})
}

func (s *Server) handlePersonaPost(w http.ResponseWriter, r *http.Request) {
	if s.Butler == nil || s.Butler.Persona() == nil {
		writeError(w, http.StatusNotImplemented, "persona not available")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	r.Body.Close()
	var req struct {
		ID       string         `json:"id"`
		Persona  butler.PersonaConfig `json:"persona"`
		Default  string         `json:"default"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	ps := s.Butler.Persona()
	if req.ID != "" && req.Persona.SystemPrompt != "" {
		ps.Set(req.ID, req.Persona)
	}
	if req.Default != "" {
		ps.SetDefault(req.Default)
	}
	ps.Save()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleArchitectSkills(w http.ResponseWriter, _ *http.Request) {
	if s.Architect == nil || s.Architect.Registry() == nil {
		writeJSON(w, http.StatusOK, map[string]any{"skills": []any{}})
		return
	}
	reg := s.Architect.Registry()
	ids := reg.List()
	var skills []map[string]any
	for _, id := range ids {
		meta, _ := reg.Meta(id)
		skills = append(skills, map[string]any{
			"id": meta.ID, "version": meta.Version, "healthy": meta.Healthy,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": skills})
}

func (s *Server) handlePipelineExecute(w http.ResponseWriter, r *http.Request) {
	actionID := GetActionID(r.Context())
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	r.Body.Close()
	var req struct {
		Stages []architect.PipelineStage `json:"stages"`
		Input  string                    `json:"input"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.Stages) == 0 {
		writeError(w, http.StatusBadRequest, "stages required")
		return
	}
	if req.Input == "" {
		req.Input = ""
	}
	pipe := architect.NewPipeline(s.Architect.Pool(), skillGuard{s.Architect}, s.Ledger, req.Stages)
	out, err := pipe.Run(r.Context(), actionID, req.Input, s.Architect.Registry())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleArchitectCrons(w http.ResponseWriter, _ *http.Request) {
	if s.Architect == nil || s.Architect.Crons() == nil {
		writeJSON(w, http.StatusOK, map[string]any{"crons": []any{}})
		return
	}
	jobs := s.Architect.Crons().List()
	writeJSON(w, http.StatusOK, map[string]any{"crons": jobs})
}

func (s *Server) handleVikingSnapshots(w http.ResponseWriter, _ *http.Request) {
	if s.VikingSnap == nil {
		writeJSON(w, http.StatusOK, map[string]any{"snapshots": []string{}})
		return
	}
	list := s.VikingSnap.ListSnapshots()
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": list})
}

func (s *Server) handleVikingSearch(w http.ResponseWriter, r *http.Request) {
	if s.VikingSearch == nil {
		writeJSON(w, http.StatusOK, map[string]any{"results": []string{}})
		return
	}
	query := r.URL.Query().Get("q")
	if query == "" && r.Method == http.MethodPost {
		var req struct {
			Query string `json:"query"`
		}
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		json.Unmarshal(body, &req)
		query = req.Query
	}
	if query == "" {
		writeError(w, http.StatusBadRequest, "query required")
		return
	}
	ids := s.VikingSearch.Search(query)
	writeJSON(w, http.StatusOK, map[string]any{"results": ids})
}

// --- request/response types ---

type engramRequest struct {
	Text           string `json:"text"`
	Source         string `json:"source"`
	Classification string `json:"classification"`
}

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
	ActionID string       `json:"action_id,omitempty"`
	Choices  []chatChoice `json:"choices"`
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
