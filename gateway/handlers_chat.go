// Package gateway provides chat and skills handlers.
package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"harmonclaw/architect"
	"harmonclaw/governor/ironclaw"
	"harmonclaw/llm"
	"harmonclaw/skills"
	"harmonclaw/viking"
)

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

	recordSkillCall(skillID)
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
