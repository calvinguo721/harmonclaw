// Package gateway provides HTTP handlers for HarmonClaw API endpoints.
package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"harmonclaw/architect"
	"harmonclaw/governor"
	"harmonclaw/llm"
	"harmonclaw/skills"
	"harmonclaw/viking"
)

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

type skillGuard struct {
	*architect.Architect
}

func (g skillGuard) CheckSkill(id string) (bool, string) {
	return g.Architect.CheckSkill(id)
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
		writeError(w, r, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()
	var req struct {
		Mode    string   `json:"mode"`
		Domains []string `json:"domains"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Mode == "" {
		req.Mode = "airlock"
	}
	validModes := map[string]bool{"shadow": true, "airlock": true, "opensea": true, "personal": true, "local": true, "connected": true}
	if !validModes[req.Mode] {
		writeError(w, r, http.StatusBadRequest, "invalid mode: "+req.Mode)
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

func (s *Server) handleRateLimitGet(w http.ResponseWriter, _ *http.Request) {
	if s.RateLimiter == nil {
		writeJSON(w, http.StatusOK, map[string]any{"global": map[string]any{"rate": 100, "burst": 200}, "per_user": map[string]any{"rate": 10, "burst": 20}, "per_skill": map[string]any{"rate": 5, "burst": 10}})
		return
	}
	cfg, _ := governor.LoadRateLimitConfig("configs/ratelimit.json")
	writeJSON(w, http.StatusOK, map[string]any{
		"global":   map[string]any{"rate": cfg.Global.Rate, "burst": cfg.Global.Burst},
		"per_user": map[string]any{"rate": cfg.PerUser.Rate, "burst": cfg.PerUser.Burst},
		"per_skill": map[string]any{"rate": cfg.PerSkill.Rate, "burst": cfg.PerSkill.Burst},
	})
}

func (s *Server) handleRateLimitPut(w http.ResponseWriter, r *http.Request) {
	if s.RateLimiter == nil {
		writeError(w, r, http.StatusServiceUnavailable, "rate limiter not configured")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "failed to read body")
		return
	}
	r.Body.Close()
	var req struct {
		Global   *struct{ Rate float64 `json:"rate"`; Burst int `json:"burst"` } `json:"global"`
		PerUser  *struct{ Rate float64 `json:"rate"`; Burst int `json:"burst"` } `json:"per_user"`
		PerSkill *struct{ Rate float64 `json:"rate"`; Burst int `json:"burst"` } `json:"per_skill"`
	}
	if json.Unmarshal(body, &req) != nil {
		writeError(w, r, http.StatusBadRequest, "invalid JSON")
		return
	}
	cfg, _ := governor.LoadRateLimitConfig("configs/ratelimit.json")
	if req.Global != nil {
		cfg.Global.Rate, cfg.Global.Burst = req.Global.Rate, req.Global.Burst
	}
	if req.PerUser != nil {
		cfg.PerUser.Rate, cfg.PerUser.Burst = req.PerUser.Rate, req.PerUser.Burst
	}
	if req.PerSkill != nil {
		cfg.PerSkill.Rate, cfg.PerSkill.Burst = req.PerSkill.Rate, req.PerSkill.Burst
	}
	s.RateLimiter.UpdateConfig(cfg)
	if s.Ledger != nil {
		s.Ledger.Record(viking.LedgerEntry{
			OperatorID: "gateway",
			ActionType: "ratelimit_update",
			Resource:   "governor",
			Result:     "ok",
			Timestamp:  time.Now().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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

func (s *Server) handleEngram(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	var req engramRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Text == "" {
		writeError(w, r, http.StatusBadRequest, "text is required")
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
		writeError(w, r, http.StatusInternalServerError, "engram path: "+err.Error())
		return
	}

	content := fmt.Sprintf("# source=%s\n# action_id=%s\n\n%s", req.Source, actionID, req.Text)
	if _, err := viking.SafeWrite(path, []byte(content), req.Classification); err != nil {
		writeError(w, r, http.StatusInternalServerError, "engram write: "+err.Error())
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
		writeError(w, r, http.StatusInternalServerError, "failed to read ledger")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleLedgerTrace(w http.ResponseWriter, r *http.Request) {
	actionID := r.URL.Query().Get("action_id")
	if actionID == "" {
		writeError(w, r, http.StatusBadRequest, "action_id query parameter required")
		return
	}
	entries, err := s.Ledger.TraceByActionID(actionID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to trace ledger")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"action_id": actionID,
		"chain":     entries,
	})
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "POST only")
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
	token, err := governor.GenerateToken(userID, "user")
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "token generation failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token, "user_id": userID})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "POST only")
		return
	}
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if json.Unmarshal(body, &req) != nil || req.Username == "" {
		writeError(w, r, http.StatusBadRequest, "username required")
		return
	}
	userID := req.Username
	token, err := governor.GenerateToken(userID, "user")
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "token generation failed")
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
