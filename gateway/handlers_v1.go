// Package gateway provides v1 API handlers (audit, persona, architect, viking).
package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"harmonclaw/architect"
	"harmonclaw/butler"
	"harmonclaw/governor"
)

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
		ID      string              `json:"id"`
		Persona butler.PersonaConfig `json:"persona"`
		Default string              `json:"default"`
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
