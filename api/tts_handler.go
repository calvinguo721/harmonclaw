// Package api holds HTTP handlers shared by the gateway (e.g. OpenAI-compatible TTS).
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"harmonclaw/skills"
)

// TTSRequest is OpenAI-compatible speech request (subset of fields).
type TTSRequest struct {
	Model string  `json:"model"`
	Input string  `json:"input"`
	Voice string  `json:"voice"`
	Speed float64 `json:"speed,omitempty"`
}

// NewEdgeTTSSpeechHandler serves POST /v1/audio/speech (audio/mpeg).
func NewEdgeTTSSpeechHandler(tts *skills.EdgeTTSSkill) http.HandlerFunc {
	if tts == nil {
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "edge tts not configured", http.StatusServiceUnavailable)
		}
	}
	aliases := skills.EdgeTTSAliases()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var req TTSRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Input == "" {
			http.Error(w, "input is required", http.StatusBadRequest)
			return
		}
		voice := skills.MapOpenAIOrAliasVoice(req.Voice, aliases)

		data, err := tts.Synthesize(r.Context(), req.Input, voice)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	}
}
