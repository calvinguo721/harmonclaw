package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"harmonclaw/llm"
	"harmonclaw/sandbox"
	"harmonclaw/viking"
)

type Router interface {
	Register(mux *http.ServeMux)
}

type Server struct {
	Addr    string
	Mux     *http.ServeMux
	LLM     llm.Provider
	Viking  viking.Memory
	Sandbox sandbox.Guard
}

func New(addr string, provider llm.Provider, mem viking.Memory, guard sandbox.Guard) *Server {
	s := &Server{
		Addr:    addr,
		Mux:     http.NewServeMux(),
		LLM:     provider,
		Viking:  mem,
		Sandbox: guard,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.Mux.HandleFunc("GET /v1/health", s.handleHealth)
	s.Mux.HandleFunc("POST /v1/chat/completions", s.handleChat)
	s.Mux.HandleFunc("POST /v1/skills/execute", s.handleSkills)
	s.Mux.HandleFunc("POST /v1/engram/inject", s.handleEngram)
}

func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.Addr, s.Mux)
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

	resp, err := s.LLM.Chat(req)
	if err != nil {
		log.Printf("llm chat error: %v", err)
		writeError(w, http.StatusBadGateway, "llm upstream error")
		return
	}

	sessionID := fmt.Sprintf("%d", time.Now().UnixMilli())
	const user = "default"

	if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1]
		if err := s.Viking.SaveMemory(user, sessionID, last.Role, last.Content); err != nil {
			log.Printf("viking save user msg: %v", err)
		}
	}
	if err := s.Viking.SaveMemory(user, sessionID, "assistant", resp.Content); err != nil {
		log.Printf("viking save assistant msg: %v", err)
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

	allowed, verdict := s.Sandbox.CheckSkill(req.SkillID)

	if !allowed {
		log.Printf("sandbox BLOCKED skill=%q verdict=%q", req.SkillID, verdict)
		writeJSON(w, http.StatusForbidden, blockResponse{
			Error:     "BLOCKED",
			RiskLevel: "CRITICAL",
			Reason:    verdict,
		})
		return
	}

	log.Printf("sandbox APPROVED skill=%q", req.SkillID)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "executed",
		"result": "All systems nominal",
	})
}

func (s *Server) handleEngram(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "Engram Bus: awaiting DeepSeek V4 activation", http.StatusNotImplemented)
}

// --- request/response types ---

type skillRequest struct {
	SkillID string `json:"skill_id"`
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
